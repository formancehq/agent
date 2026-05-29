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
	defaultPageSize    = 100
	heartbeatInterval  = 30 * time.Second
	expectedStatusSync = "active"
	expectedStatusDel  = "deleted"
)

type pollingClient struct {
	agentClient  generated.AgentServiceClient
	reconciler   *MembershipListener
	k8sClient    K8SClient
	clientInfo   ClientInfo
	modules      modules
	eeModules    eeModules
	pollInterval time.Duration
	cursor       string // persistent cursor across polls
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

	// Send initial heartbeat immediately
	if err := p.sendHeartbeat(ctx); err != nil {
		logger.Errorf("Initial heartbeat failed: %s", err)
	}

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := p.sendHeartbeat(ctx); err != nil {
				logger.Errorf("Heartbeat failed: %s", err)
			}
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

	// First poll: full sync with orphan cleanup
	isFirstPoll := p.cursor == ""
	if err := p.poll(ctx, isFirstPoll); err != nil {
		logger.Errorf("Initial poll failed: %s", err)
	}

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := p.poll(ctx, false); err != nil {
				logger.Errorf("Poll failed: %s", err)
			}
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

	// Orphan cleanup on first poll (full sync)
	if isFullSync {
		p.cleanupOrphans(ctx, allStackNames)
	}

	return nil
}

func (p *pollingClient) reconcileStack(ctx context.Context, stack *generated.Stack) {
	logger := logging.FromContext(ctx)

	switch stack.GetExpectedStatus() {
	case expectedStatusDel:
		logger.Infof("Stack %s expected to be deleted, deleting", stack.ClusterName)
		p.reconciler.DeleteStack(ctx, &generated.DeletedStack{
			ClusterName: stack.ClusterName,
		})
	case expectedStatusSync, "":
		if stack.GetDisabled() {
			logger.Infof("Stack %s is disabled, disabling", stack.ClusterName)
			p.reconciler.DisableStack(ctx, stack.ClusterName)
		} else {
			logger.Infof("Syncing existing stack %s", stack.ClusterName)
			p.reconciler.SyncExistingStack(ctx, stack)
		}
	default:
		logger.Infof("Unknown expected status %q for stack %s, syncing", stack.GetExpectedStatus(), stack.ClusterName)
		p.reconciler.SyncExistingStack(ctx, stack)
	}
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
	eeModules eeModules,
	pollInterval time.Duration,
) *pollingClient {
	return &pollingClient{
		agentClient:  agentClient,
		reconciler:   reconciler,
		k8sClient:    k8sClient,
		clientInfo:   clientInfo,
		modules:      modules,
		eeModules:    eeModules,
		pollInterval: pollInterval,
	}
}
