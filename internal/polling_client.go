package internal

import (
	"context"
	"time"

	"github.com/formancehq/go-libs/v2/logging"
	"github.com/formancehq/stack/components/agent/internal/generated"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

const (
	defaultPageSize   = 100
	heartbeatInterval = 30 * time.Second
	expectedStatusDel = "deleted"
)

type pollingClient struct {
	agentClient  generated.AgentServiceClient
	reconciler   *MembershipListener
	k8sClient    K8SClient
	clientInfo   ClientInfo
	modules      modules
	pollInterval time.Duration
	cursor       string // persistent cursor across polls
	fullSyncDone bool   // tracks whether first full sync with orphan cleanup succeeded
}

func (p *pollingClient) Start(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	// Heartbeat goroutine
	g.Go(func() error {
		return p.runHeartbeat(ctx)
	})

	// Poll loop goroutine
	g.Go(func() error {
		return p.runPollLoop(ctx)
	})

	return g.Wait()
}

func (p *pollingClient) runHeartbeat(ctx context.Context) error {
	logger := logging.FromContext(ctx)

	hbBackoff := heartbeatInterval
	maxBackoff := heartbeatInterval * 16

	// Send initial heartbeat immediately
	if err := p.sendHeartbeat(ctx); err != nil {
		logger.Errorf("Initial heartbeat failed: %s", err)
		hbBackoff = min(hbBackoff*2, maxBackoff)
	}

	ticker := time.NewTicker(hbBackoff)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := p.sendHeartbeat(ctx); err != nil {
				logger.Errorf("Heartbeat failed: %s", err)
				hbBackoff = min(hbBackoff*2, maxBackoff)
			} else {
				hbBackoff = heartbeatInterval
			}
			ticker.Reset(hbBackoff)
		}
	}
}

func (p *pollingClient) sendHeartbeat(ctx context.Context) error {
	_, err := p.agentClient.Heartbeat(ctx, &generated.HeartbeatRequest{
		RegionId:           p.clientInfo.ID,
		BaseUrl:            p.clientInfo.BaseUrl.String(),
		AdditionalBaseUrls: p.clientInfo.AdditionalBaseURLs,
		Version:            p.clientInfo.Version,
		Production:         p.clientInfo.Production,
		Capabilities:       []string{capabilityEE, capabilityModuleList},
		Modules:            p.modules.Singular(),
	})
	return err
}

func (p *pollingClient) runPollLoop(ctx context.Context) error {
	logger := logging.FromContext(ctx)

	pollBackoff := p.pollInterval
	maxBackoff := p.pollInterval * 16

	// First poll: full sync with orphan cleanup
	if err := p.poll(ctx, !p.fullSyncDone); err != nil {
		logger.Errorf("Initial poll failed: %s", err)
		pollBackoff = min(pollBackoff*2, maxBackoff)
	} else {
		pollBackoff = p.pollInterval
	}

	ticker := time.NewTicker(pollBackoff)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := p.poll(ctx, !p.fullSyncDone); err != nil {
				logger.Errorf("Poll failed: %s", err)
				pollBackoff = min(pollBackoff*2, maxBackoff)
			} else {
				pollBackoff = p.pollInterval
			}
			ticker.Reset(pollBackoff)
		}
	}
}

func (p *pollingClient) poll(ctx context.Context, isFullSync bool) error {
	logger := logging.FromContext(ctx)
	logger.Infof("Polling membership for stack changes (cursor=%q, fullSync=%t)", p.cursor, isFullSync)

	// Collect all stacks from this poll for orphan detection
	var allStackNames map[string]struct{}
	if isFullSync {
		allStackNames = make(map[string]struct{})
	}

	cursor := p.cursor
	for {
		resp, err := p.agentClient.ListStacks(ctx, &generated.ListStacksRequest{
			RegionId: p.clientInfo.ID,
			PageSize: defaultPageSize,
			Cursor:   cursor,
		})
		if err != nil {
			return err
		}

		for _, stack := range resp.GetStacks() {
			logger := logger.WithField("stack", stack.ClusterName)
			ctx := logging.ContextWithLogger(ctx, logger)

			if isFullSync {
				allStackNames[stack.ClusterName] = struct{}{}
			}

			p.reconcileStack(ctx, stack)
		}

		if !resp.GetHasMore() {
			p.cursor = resp.GetNextCursor()
			break
		}
		cursor = resp.GetNextCursor()
	}

	// Orphan cleanup on full sync
	if isFullSync {
		p.cleanupOrphans(ctx, allStackNames)
		p.fullSyncDone = true
	}

	return nil
}

func (p *pollingClient) reconcileStack(ctx context.Context, stack *generated.Stack) {
	logger := logging.FromContext(ctx)

	if stack.GetExpectedStatus() == expectedStatusDel {
		logger.Infof("Stack %s expected to be deleted, deleting", stack.ClusterName)
		p.reconciler.DeleteStack(ctx, &generated.DeletedStack{
			ClusterName: stack.ClusterName,
		})
		return
	}

	// SyncExistingStack handles both active and disabled stacks (disabled: true in spec)
	logger.Infof("Syncing existing stack %s (disabled=%t)", stack.ClusterName, stack.GetDisabled())
	p.reconciler.SyncExistingStack(ctx, stack)
}

func (p *pollingClient) cleanupOrphans(ctx context.Context, knownStacks map[string]struct{}) {
	logger := logging.FromContext(ctx)
	logger.Infof("Running orphan cleanup, known stacks: %d", len(knownStacks))

	// List all K8s stacks with the agent label
	agentLabel, err := labels.NewRequirement("formance.com/created-by-agent", selection.Equals, []string{"true"})
	if err != nil {
		logger.Errorf("Failed to create label requirement: %s", err)
		return
	}

	selector := labels.NewSelector().Add(*agentLabel)
	k8sStacks, err := p.k8sClient.List(ctx, "Stacks", selector)
	if err != nil {
		logger.Errorf("Failed to list K8s stacks for orphan cleanup: %s", err)
		return
	}

	for _, k8sStack := range k8sStacks {
		name := k8sStack.GetName()
		if _, known := knownStacks[name]; !known {
			logger.Infof("Cleaning up orphan stack %s", name)
			if err := p.k8sClient.Delete(ctx, "Stacks", name); err != nil {
				logger.Errorf("Failed to delete orphan stack %s: %s", name, err)
			}
		}
	}
}

func NewPollingClient(
	agentClient generated.AgentServiceClient,
	reconciler *MembershipListener,
	k8sClient K8SClient,
	clientInfo ClientInfo,
	modules modules,
	pollInterval time.Duration,
) *pollingClient {
	return &pollingClient{
		agentClient:  agentClient,
		reconciler:   reconciler,
		k8sClient:    k8sClient,
		clientInfo:   clientInfo,
		modules:      modules,
		pollInterval: pollInterval,
	}
}
