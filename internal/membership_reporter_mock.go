package internal

import (
	"context"
	"sync"

	"github.com/formancehq/stack/components/agent/internal/generated"
	"google.golang.org/protobuf/types/known/structpb"
)

// ReportedEvent represents an event captured by the mock reporter for test assertions.
type ReportedEvent struct {
	Type        string
	ClusterName string
	Statuses    *structpb.Struct
	Vk          *generated.VersionKind
	Status      *structpb.Struct
	Name        string
	Versions    map[string]string
	Deprecated  bool
}

type MembershipReporterMock struct {
	mu     sync.Mutex
	events []ReportedEvent
}

func (m *MembershipReporterMock) ReportStackStatus(_ context.Context, clusterName string, statuses *structpb.Struct) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.events = append(m.events, ReportedEvent{
		Type:        "StackStatus",
		ClusterName: clusterName,
		Statuses:    statuses,
	})
	return nil
}

func (m *MembershipReporterMock) ReportStackDeleted(_ context.Context, clusterName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.events = append(m.events, ReportedEvent{
		Type:        "StackDeleted",
		ClusterName: clusterName,
	})
	return nil
}

func (m *MembershipReporterMock) ReportModuleStatus(_ context.Context, clusterName string, vk *generated.VersionKind, status *structpb.Struct) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.events = append(m.events, ReportedEvent{
		Type:        "ModuleStatus",
		ClusterName: clusterName,
		Vk:          vk,
		Status:      status,
	})
	return nil
}

func (m *MembershipReporterMock) ReportModuleDeleted(_ context.Context, clusterName string, vk *generated.VersionKind) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.events = append(m.events, ReportedEvent{
		Type:        "ModuleDeleted",
		ClusterName: clusterName,
		Vk:          vk,
	})
	return nil
}

func (m *MembershipReporterMock) UpsertVersion(_ context.Context, name string, versions map[string]string, deprecated bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.events = append(m.events, ReportedEvent{
		Type:       "UpsertVersion",
		Name:       name,
		Versions:   versions,
		Deprecated: deprecated,
	})
	return nil
}

func (m *MembershipReporterMock) DeleteVersion(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.events = append(m.events, ReportedEvent{
		Type: "DeleteVersion",
		Name: name,
	})
	return nil
}

func (m *MembershipReporterMock) GetEvents() []ReportedEvent {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]ReportedEvent, len(m.events))
	copy(result, m.events)
	return result
}

func NewMembershipReporterMock() *MembershipReporterMock {
	return &MembershipReporterMock{}
}
