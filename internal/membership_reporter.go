package internal

import (
	"context"

	"github.com/formancehq/stack/components/agent/internal/generated"
	"google.golang.org/protobuf/types/known/structpb"
)

// MembershipReporter reports observed K8s state back to membership via unary RPCs.
type MembershipReporter interface {
	ReportStackStatus(ctx context.Context, clusterName string, statuses *structpb.Struct) error
	ReportStackDeleted(ctx context.Context, clusterName string) error
	ReportModuleStatus(ctx context.Context, clusterName string, vk *generated.VersionKind, status *structpb.Struct) error
	ReportModuleDeleted(ctx context.Context, clusterName string, vk *generated.VersionKind) error
	UpsertVersion(ctx context.Context, name string, versions map[string]string, deprecated bool) error
	DeleteVersion(ctx context.Context, name string) error
}

type membershipReporter struct {
	client generated.AgentServiceClient
}

func (r *membershipReporter) ReportStackStatus(ctx context.Context, clusterName string, statuses *structpb.Struct) error {
	_, err := r.client.ReportStackStatus(ctx, &generated.ReportStackStatusRequest{
		StatusChanged: &generated.StatusChanged{
			ClusterName: clusterName,
			Statuses:    statuses,
		},
	})
	return err
}

func (r *membershipReporter) ReportStackDeleted(ctx context.Context, clusterName string) error {
	_, err := r.client.ReportStackDeleted(ctx, &generated.ReportStackDeletedRequest{
		StackDeleted: &generated.DeletedStack{
			ClusterName: clusterName,
		},
	})
	return err
}

func (r *membershipReporter) ReportModuleStatus(ctx context.Context, clusterName string, vk *generated.VersionKind, status *structpb.Struct) error {
	_, err := r.client.ReportModuleStatus(ctx, &generated.ReportModuleStatusRequest{
		ModuleStatusChanged: &generated.ModuleStatusChanged{
			ClusterName: clusterName,
			Vk:          vk,
			Status:      status,
		},
	})
	return err
}

func (r *membershipReporter) ReportModuleDeleted(ctx context.Context, clusterName string, vk *generated.VersionKind) error {
	_, err := r.client.ReportModuleDeleted(ctx, &generated.ReportModuleDeletedRequest{
		ModuleDeleted: &generated.ModuleDeleted{
			ClusterName: clusterName,
			Vk:          vk,
		},
	})
	return err
}

func (r *membershipReporter) UpsertVersion(ctx context.Context, name string, versions map[string]string, deprecated bool) error {
	_, err := r.client.UpsertVersion(ctx, &generated.UpsertVersionRequest{
		Name:       name,
		Versions:   versions,
		Deprecated: deprecated,
	})
	return err
}

func (r *membershipReporter) DeleteVersion(ctx context.Context, name string) error {
	_, err := r.client.DeleteVersion(ctx, &generated.DeleteVersionRequest{
		Name: name,
	})
	return err
}

func NewMembershipReporter(client generated.AgentServiceClient) MembershipReporter {
	return &membershipReporter{
		client: client,
	}
}
