package internal

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	v1apis "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"github.com/formancehq/go-libs/v2/logging"
	"github.com/formancehq/stack/components/agent/internal/generated"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func newTestListener(k8s K8SClient, membership MembershipClient, modules []v1apis.CustomResourceDefinition) *membershipListener {
	return NewMembershipListener(
		k8s,
		ClientInfo{
			BaseUrl: &url.URL{Scheme: "https", Host: "example.com"},
		},
		nil, // restMapper not needed when we mock Apply directly
		membership,
		modules,
	)
}

func newTestListenerWithMapper(k8s K8SClient, membership MembershipClient, modules []v1apis.CustomResourceDefinition, mapper *fakeRESTMapper) *membershipListener {
	return NewMembershipListener(
		k8s,
		ClientInfo{
			BaseUrl: &url.URL{Scheme: "https", Host: "example.com"},
		},
		mapper,
		membership,
		modules,
	)
}

// fakeRESTMapper satisfies meta.RESTMapper for unit tests.
type fakeRESTMapper struct {
	mappings map[schema.GroupKind]*fakeRESTMapping
}

type fakeRESTMapping struct {
	resource string
}

func (f *fakeRESTMapper) KindFor(_ schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, fmt.Errorf("not implemented")
}
func (f *fakeRESTMapper) KindsFor(_ schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeRESTMapper) ResourceFor(_ schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	return schema.GroupVersionResource{}, fmt.Errorf("not implemented")
}
func (f *fakeRESTMapper) ResourcesFor(_ schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeRESTMapper) RESTMapping(gk schema.GroupKind, _ ...string) (*meta.RESTMapping, error) {
	m, ok := f.mappings[gk]
	if !ok {
		return nil, fmt.Errorf("no mapping for %s", gk)
	}
	return &meta.RESTMapping{
		Resource: schema.GroupVersionResource{
			Group:    gk.Group,
			Version:  "v1beta1",
			Resource: m.resource,
		},
		GroupVersionKind: schema.GroupVersionKind{
			Group:   gk.Group,
			Version: "v1beta1",
			Kind:    gk.Kind,
		},
	}, nil
}
func (f *fakeRESTMapper) RESTMappings(_ schema.GroupKind, _ ...string) ([]*meta.RESTMapping, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeRESTMapper) ResourceSingularizer(_ string) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func defaultMapper() *fakeRESTMapper {
	return &fakeRESTMapper{
		mappings: map[schema.GroupKind]*fakeRESTMapping{
			{Group: "formance.com", Kind: "Stack"}:      {resource: "stacks"},
			{Group: "formance.com", Kind: "Auth"}:        {resource: "auths"},
			{Group: "formance.com", Kind: "Gateway"}:     {resource: "gateways"},
			{Group: "formance.com", Kind: "Ledger"}:      {resource: "ledgers"},
			{Group: "formance.com", Kind: "Stargate"}:    {resource: "stargates"},
			{Group: "formance.com", Kind: "AuthClient"}:  {resource: "authclients"},
			{Group: "formance.com", Kind: "Webhooks"}:    {resource: "webhooks"},
			{Group: "formance.com", Kind: "Payments"}:    {resource: "payments"},
			{Group: "formance.com", Kind: "Search"}:      {resource: "searches"},
			{Group: "formance.com", Kind: "Orchestration"}: {resource: "orchestrations"},
			{Group: "formance.com", Kind: "Wallets"}:     {resource: "wallets"},
			{Group: "formance.com", Kind: "Reconciliation"}: {resource: "reconciliations"},
		},
	}
}

func newFakeStack(name string) *unstructured.Unstructured {
	s := &unstructured.Unstructured{}
	s.SetName(name)
	s.SetUID("fake-uid")
	return s
}

// --- syncExistingStack error paths ---

func TestSyncExistingStack_ApplyFailure(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	applyErr := fmt.Errorf("connection refused")
	k8s := &mockK8SClient{
		applyFn: func(_ context.Context, _ string, _ *unstructured.Unstructured, _ string, _ bool) (*unstructured.Unstructured, error) {
			return nil, applyErr
		},
	}
	listener := newTestListenerWithMapper(k8s, NewMembershipClientMock(), nil, defaultMapper())

	// Should not panic; error is logged, sync stops before modules
	listener.syncExistingStack(ctx, &generated.Stack{
		ClusterName: uuid.NewString(),
		AuthConfig:  &generated.AuthConfig{},
	})
}

func TestSyncExistingStack_VersionsDefault(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	var appliedContent map[string]any
	k8s := &mockK8SClient{
		applyFn: func(_ context.Context, resource string, obj *unstructured.Unstructured, _ string, _ bool) (*unstructured.Unstructured, error) {
			if resource == "stacks" {
				appliedContent = obj.Object
			}
			result := obj.DeepCopy()
			result.SetUID("fake-uid")
			return result, nil
		},
	}
	listener := newTestListenerWithMapper(k8s, NewMembershipClientMock(), nil, defaultMapper())

	listener.syncExistingStack(ctx, &generated.Stack{
		ClusterName: uuid.NewString(),
		Versions:    "",
		AuthConfig:  &generated.AuthConfig{},
	})

	spec, _ := appliedContent["spec"].(map[string]any)
	require.Equal(t, "default", spec["versionsFromFile"])
}

func TestSyncExistingStack_ExplicitVersions(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	var appliedContent map[string]any
	k8s := &mockK8SClient{
		applyFn: func(_ context.Context, resource string, obj *unstructured.Unstructured, _ string, _ bool) (*unstructured.Unstructured, error) {
			if resource == "stacks" {
				appliedContent = obj.Object
			}
			result := obj.DeepCopy()
			result.SetUID("fake-uid")
			return result, nil
		},
	}
	listener := newTestListenerWithMapper(k8s, NewMembershipClientMock(), nil, defaultMapper())

	listener.syncExistingStack(ctx, &generated.Stack{
		ClusterName: uuid.NewString(),
		Versions:    "v1.2.3",
		AuthConfig:  &generated.AuthConfig{},
	})

	spec, _ := appliedContent["spec"].(map[string]any)
	require.Equal(t, "v1.2.3", spec["versionsFromFile"])
}

// --- syncModules error paths ---

func testModuleCRDs() []v1apis.CustomResourceDefinition {
	makeCRD := func(kind, singular, plural string) v1apis.CustomResourceDefinition {
		return v1apis.CustomResourceDefinition{
			Spec: v1apis.CustomResourceDefinitionSpec{
				Group: "formance.com",
				Names: v1apis.CustomResourceDefinitionNames{Kind: kind},
				Versions: []v1apis.CustomResourceDefinitionVersion{
					{Name: "v1beta1"},
				},
			},
			Status: v1apis.CustomResourceDefinitionStatus{
				AcceptedNames: v1apis.CustomResourceDefinitionNames{
					Singular: singular,
					Plural:   plural,
				},
			},
		}
	}
	return []v1apis.CustomResourceDefinition{
		makeCRD("Auth", "auth", "auths"),
		makeCRD("Gateway", "gateway", "gateways"),
		makeCRD("Ledger", "ledger", "ledgers"),
	}
}

func TestSyncModules_PartialApplyFailure(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	var appliedResources []string
	k8s := &mockK8SClient{
		applyFn: func(_ context.Context, resource string, obj *unstructured.Unstructured, _ string, _ bool) (*unstructured.Unstructured, error) {
			appliedResources = append(appliedResources, resource)
			if resource == "auths" {
				return nil, fmt.Errorf("auth apply failed")
			}
			result := obj.DeepCopy()
			result.SetUID("fake-uid")
			return result, nil
		},
	}

	modules := testModuleCRDs()
	listener := newTestListenerWithMapper(k8s, NewMembershipClientMock(), modules, defaultMapper())

	stack := newFakeStack("test-stack")
	membershipStack := &generated.Stack{
		ClusterName: "test-stack",
		AuthConfig:  &generated.AuthConfig{},
		Modules: []*generated.Module{
			{Name: "Auth"},
			{Name: "Gateway"},
			{Name: "Ledger"},
		},
	}

	// Should not panic; Auth fails but Gateway and Ledger are still attempted
	listener.syncModules(ctx, map[string]any{}, stack, membershipStack)

	assert.Contains(t, appliedResources, "auths")
	assert.Contains(t, appliedResources, "gateways")
	assert.Contains(t, appliedResources, "ledgers")
}

func TestSyncModules_DeleteModuleFailure(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	deleteErr := fmt.Errorf("delete forbidden")
	k8s := &mockK8SClient{
		ensureNotExistsBySelectorFn: func(_ context.Context, _ string, _ labels.Selector) error {
			return deleteErr
		},
	}

	modules := testModuleCRDs()
	listener := newTestListenerWithMapper(k8s, NewMembershipClientMock(), modules, defaultMapper())

	stack := newFakeStack("test-stack")
	membershipStack := &generated.Stack{
		ClusterName: "test-stack",
		AuthConfig:  &generated.AuthConfig{},
		Modules:     []*generated.Module{}, // no modules expected → all should be deleted
	}

	// Should not panic; errors logged, loop continues
	listener.syncModules(ctx, map[string]any{}, stack, membershipStack)
}

// --- syncStargate error paths ---

func TestSyncStargate_ApplyFailure(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	k8s := &mockK8SClient{
		applyFn: func(_ context.Context, _ string, _ *unstructured.Unstructured, _ string, _ bool) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("stargate apply failed")
		},
	}
	listener := newTestListenerWithMapper(k8s, NewMembershipClientMock(), nil, defaultMapper())

	stack := newFakeStack("org-stack")
	membershipStack := &generated.Stack{
		AuthConfig:     &generated.AuthConfig{},
		StargateConfig: &generated.StargateConfig{Enabled: true},
	}

	// Should not panic
	listener.syncStargate(ctx, map[string]any{}, stack, membershipStack)
}

func TestSyncStargate_DeleteFailure(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	k8s := &mockK8SClient{
		ensureNotExistsFn: func(_ context.Context, _, _ string) error {
			return fmt.Errorf("delete failed")
		},
	}
	listener := newTestListenerWithMapper(k8s, NewMembershipClientMock(), nil, defaultMapper())

	stack := newFakeStack("org-stack")
	membershipStack := &generated.Stack{
		StargateConfig: &generated.StargateConfig{Enabled: false},
	}

	// Should not panic
	listener.syncStargate(ctx, map[string]any{}, stack, membershipStack)
}

func TestSyncStargate_NilConfig(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	var deleteCalled bool
	k8s := &mockK8SClient{
		ensureNotExistsFn: func(_ context.Context, _, _ string) error {
			deleteCalled = true
			return nil
		},
	}
	listener := newTestListenerWithMapper(k8s, NewMembershipClientMock(), nil, defaultMapper())

	stack := newFakeStack("org-stack")
	membershipStack := &generated.Stack{
		StargateConfig: nil,
	}

	listener.syncStargate(ctx, map[string]any{}, stack, membershipStack)
	assert.True(t, deleteCalled, "should attempt to delete stargate when config is nil")
}

// --- syncAuthClients error paths ---

func TestSyncAuthClients_ApplyFailure(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	callCount := 0
	k8s := &mockK8SClient{
		applyFn: func(_ context.Context, _ string, obj *unstructured.Unstructured, _ string, _ bool) (*unstructured.Unstructured, error) {
			callCount++
			if callCount == 1 {
				return nil, fmt.Errorf("apply failed for first client")
			}
			result := obj.DeepCopy()
			result.SetUID("fake-uid")
			return result, nil
		},
	}
	listener := newTestListenerWithMapper(k8s, NewMembershipClientMock(), nil, defaultMapper())

	stack := newFakeStack("test-stack")
	clients := []*generated.AuthClient{
		{Id: "client1", Public: true},
		{Id: "client2", Public: false},
	}

	// Should not panic; first client fails, second succeeds
	listener.syncAuthClients(ctx, map[string]any{}, stack, clients)
	assert.Equal(t, 2, callCount, "should attempt apply for both clients")
}

func TestSyncAuthClients_ListFailure(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	k8s := &mockK8SClient{
		applyFn: func(_ context.Context, _ string, obj *unstructured.Unstructured, _ string, _ bool) (*unstructured.Unstructured, error) {
			result := obj.DeepCopy()
			result.SetUID("fake-uid")
			return result, nil
		},
		listFn: func(_ context.Context, _ string, _ labels.Selector) ([]unstructured.Unstructured, error) {
			return nil, fmt.Errorf("list forbidden")
		},
	}
	listener := newTestListenerWithMapper(k8s, NewMembershipClientMock(), nil, defaultMapper())

	stack := newFakeStack("test-stack")
	clients := []*generated.AuthClient{
		{Id: "client1", Public: true},
	}

	// Should not panic; list fails so stale clients are not cleaned up
	listener.syncAuthClients(ctx, map[string]any{}, stack, clients)
}

func TestSyncAuthClients_DeleteStaleFailure(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	var deleteAttempted []string
	k8s := &mockK8SClient{
		applyFn: func(_ context.Context, _ string, obj *unstructured.Unstructured, _ string, _ bool) (*unstructured.Unstructured, error) {
			result := obj.DeepCopy()
			result.SetUID("fake-uid")
			return result, nil
		},
		listFn: func(_ context.Context, _ string, _ labels.Selector) ([]unstructured.Unstructured, error) {
			return []unstructured.Unstructured{
				{Object: map[string]any{"metadata": map[string]any{"name": "test-stack-client1"}}},
				{Object: map[string]any{"metadata": map[string]any{"name": "test-stack-stale"}}},
			}, nil
		},
		ensureNotExistsFn: func(_ context.Context, _ string, name string) error {
			deleteAttempted = append(deleteAttempted, name)
			return fmt.Errorf("delete forbidden")
		},
	}
	listener := newTestListenerWithMapper(k8s, NewMembershipClientMock(), nil, defaultMapper())

	stack := newFakeStack("test-stack")
	clients := []*generated.AuthClient{
		{Id: "client1", Public: true},
	}

	// Should not panic; stale client deletion fails but is logged
	listener.syncAuthClients(ctx, map[string]any{}, stack, clients)
	assert.Contains(t, deleteAttempted, "test-stack-stale")
}

// --- deleteStack error paths ---

func TestDeleteStack_K8SError(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	k8s := &mockK8SClient{
		deleteFn: func(_ context.Context, _, _ string) error {
			return fmt.Errorf("internal server error")
		},
	}
	mock := NewMembershipClientMock()
	listener := newTestListener(k8s, mock, nil)

	// Should not panic; error logged, no message sent to membership
	listener.deleteStack(ctx, &generated.DeletedStack{ClusterName: "broken-stack"})
	assert.Empty(t, mock.GetMessages(), "should not send any message on non-NotFound error")
}

func TestDeleteStack_SendFailure(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	k8s := &mockK8SClient{
		deleteFn: func(_ context.Context, _, _ string) error {
			return apierrors.NewNotFound(schema.GroupResource{Group: "formance.com", Resource: "stacks"}, "gone-stack")
		},
	}

	// Use a real mock that will track messages, but the test verifies no panic
	mock := NewMembershipClientMock()
	listener := newTestListener(k8s, mock, nil)

	listener.deleteStack(ctx, &generated.DeletedStack{ClusterName: "gone-stack"})
	require.Len(t, mock.GetMessages(), 1)
	assert.NotNil(t, mock.GetMessages()[0].GetStackDeleted())
}

// --- disableStack / enableStack error paths ---

func TestDisableStack_ApplyFailure(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	k8s := &mockK8SClient{
		applyFn: func(_ context.Context, _ string, _ *unstructured.Unstructured, _ string, _ bool) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("apply failed")
		},
	}
	listener := newTestListener(k8s, NewMembershipClientMock(), nil)

	// Should not panic
	listener.disableStack(ctx, &generated.DisabledStack{ClusterName: "test-stack"})
}

func TestEnableStack_ApplyFailure(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	k8s := &mockK8SClient{
		applyFn: func(_ context.Context, _ string, _ *unstructured.Unstructured, _ string, _ bool) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("apply failed")
		},
	}
	listener := newTestListener(k8s, NewMembershipClientMock(), nil)

	// Should not panic
	listener.enableStack(ctx, &generated.EnabledStack{ClusterName: "test-stack"})
}

func TestDisableStack_FieldManager(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	var capturedFieldManager string
	var capturedForce bool
	k8s := &mockK8SClient{
		applyFn: func(_ context.Context, _ string, _ *unstructured.Unstructured, fm string, force bool) (*unstructured.Unstructured, error) {
			capturedFieldManager = fm
			capturedForce = force
			return &unstructured.Unstructured{}, nil
		},
	}
	listener := newTestListener(k8s, NewMembershipClientMock(), nil)

	listener.disableStack(ctx, &generated.DisabledStack{ClusterName: "test-stack"})
	assert.Equal(t, fieldManagerDisable, capturedFieldManager)
	assert.True(t, capturedForce, "disable should use force=true")
}

func TestEnableStack_FieldManager(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	var capturedFieldManager string
	var capturedForce bool
	k8s := &mockK8SClient{
		applyFn: func(_ context.Context, _ string, _ *unstructured.Unstructured, fm string, force bool) (*unstructured.Unstructured, error) {
			capturedFieldManager = fm
			capturedForce = force
			return &unstructured.Unstructured{}, nil
		},
	}
	listener := newTestListener(k8s, NewMembershipClientMock(), nil)

	listener.enableStack(ctx, &generated.EnabledStack{ClusterName: "test-stack"})
	assert.Equal(t, fieldManagerDisable, capturedFieldManager)
	assert.True(t, capturedForce, "enable should use force=true")
}

// --- createOrUpdate error paths ---

func TestCreateOrUpdate_ConflictFallsBackToGet(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	existingObj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name": "my-stack",
				"annotations": map[string]any{
					"user/custom": "preserved",
				},
			},
		},
	}

	k8s := &mockK8SClient{
		applyFn: func(_ context.Context, _ string, _ *unstructured.Unstructured, _ string, _ bool) (*unstructured.Unstructured, error) {
			return nil, apierrors.NewConflict(schema.GroupResource{Group: "formance.com", Resource: "stacks"}, "my-stack", fmt.Errorf("field owned by kubectl-edit"))
		},
		getFn: func(_ context.Context, _ string, _ string) (*unstructured.Unstructured, error) {
			return existingObj, nil
		},
	}

	listener := newTestListenerWithMapper(k8s, NewMembershipClientMock(), nil, defaultMapper())

	result, err := listener.createOrUpdate(ctx, formanceGroupVersion.WithKind("Stack"), "my-stack", "my-stack", nil, map[string]any{
		"spec": map[string]any{"versionsFromFile": "default"},
	})

	require.NoError(t, err)
	assert.Equal(t, "preserved", result.GetAnnotations()["user/custom"])
}

func TestCreateOrUpdate_ConflictGetFailure(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	k8s := &mockK8SClient{
		applyFn: func(_ context.Context, _ string, _ *unstructured.Unstructured, _ string, _ bool) (*unstructured.Unstructured, error) {
			return nil, apierrors.NewConflict(schema.GroupResource{}, "x", fmt.Errorf("conflict"))
		},
		getFn: func(_ context.Context, _ string, _ string) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("get also failed")
		},
	}

	listener := newTestListenerWithMapper(k8s, NewMembershipClientMock(), nil, defaultMapper())

	_, err := listener.createOrUpdate(ctx, formanceGroupVersion.WithKind("Stack"), "x", "x", nil, map[string]any{
		"spec": map[string]any{},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "getting existing object after conflict")
}

func TestCreateOrUpdate_NonConflictError(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	k8s := &mockK8SClient{
		applyFn: func(_ context.Context, _ string, _ *unstructured.Unstructured, _ string, _ bool) (*unstructured.Unstructured, error) {
			return nil, fmt.Errorf("network error")
		},
	}

	listener := newTestListenerWithMapper(k8s, NewMembershipClientMock(), nil, defaultMapper())

	_, err := listener.createOrUpdate(ctx, formanceGroupVersion.WithKind("Stack"), "x", "x", nil, map[string]any{
		"spec": map[string]any{},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "applying object")
}

func TestCreateOrUpdate_FieldManagerIsAgent(t *testing.T) {
	t.Parallel()
	ctx := logging.TestingContext()

	var capturedFM string
	var capturedForce bool
	k8s := &mockK8SClient{
		applyFn: func(_ context.Context, _ string, obj *unstructured.Unstructured, fm string, force bool) (*unstructured.Unstructured, error) {
			capturedFM = fm
			capturedForce = force
			result := obj.DeepCopy()
			result.SetUID("fake-uid")
			return result, nil
		},
	}

	listener := newTestListenerWithMapper(k8s, NewMembershipClientMock(), nil, defaultMapper())

	_, err := listener.createOrUpdate(ctx, formanceGroupVersion.WithKind("Stack"), "x", "x", nil, map[string]any{
		"spec": map[string]any{},
	})

	require.NoError(t, err)
	assert.Equal(t, fieldManagerAgent, capturedFM)
	assert.False(t, capturedForce, "createOrUpdate should use force=false")
}

// --- informer_versions (entirely untested until now) ---

func TestVersionsEventHandler_Add(t *testing.T) {
	t.Parallel()

	mock := NewMembershipClientMock()
	handler := VersionsEventHandler(logging.Testing(), mock)

	version := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name": "v2.0",
				"annotations": map[string]any{
					"formance.com/deprecated": "true",
				},
			},
			"spec": map[string]any{
				"ledger":   "v2.0.0",
				"payments": "v1.5.0",
			},
		},
	}

	handler.OnAdd(version, false)

	require.Len(t, mock.GetMessages(), 1)
	msg := mock.GetMessages()[0].GetAddedVersion()
	require.NotNil(t, msg)
	assert.Equal(t, "v2.0", msg.Name)
	assert.Equal(t, "v2.0.0", msg.Versions["ledger"])
	assert.Equal(t, "v1.5.0", msg.Versions["payments"])
	assert.True(t, msg.Deprecated)
}

func TestVersionsEventHandler_Add_NoSpec(t *testing.T) {
	t.Parallel()

	mock := NewMembershipClientMock()
	handler := VersionsEventHandler(logging.Testing(), mock)

	version := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name": "empty",
			},
		},
	}

	handler.OnAdd(version, false)

	require.Len(t, mock.GetMessages(), 1)
	msg := mock.GetMessages()[0].GetAddedVersion()
	require.NotNil(t, msg)
	assert.Nil(t, msg.Versions)
	assert.False(t, msg.Deprecated)
}

func TestVersionsEventHandler_Update_Changed(t *testing.T) {
	t.Parallel()

	mock := NewMembershipClientMock()
	handler := VersionsEventHandler(logging.Testing(), mock)

	oldVersion := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{"name": "v1"},
			"spec":     map[string]any{"ledger": "v1.0.0"},
		},
	}
	newVersion := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{"name": "v1"},
			"spec":     map[string]any{"ledger": "v1.1.0"},
		},
	}

	handler.OnUpdate(oldVersion, newVersion)

	require.Len(t, mock.GetMessages(), 1)
	msg := mock.GetMessages()[0].GetUpdatedVersion()
	require.NotNil(t, msg)
	assert.Equal(t, "v1.1.0", msg.Versions["ledger"])
}

func TestVersionsEventHandler_Update_Unchanged(t *testing.T) {
	t.Parallel()

	mock := NewMembershipClientMock()
	handler := VersionsEventHandler(logging.Testing(), mock)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{"name": "v1"},
			"spec":     map[string]any{"ledger": "v1.0.0"},
		},
	}

	handler.OnUpdate(obj, obj)

	assert.Empty(t, mock.GetMessages(), "should not send message when spec unchanged")
}

func TestVersionsEventHandler_Delete(t *testing.T) {
	t.Parallel()

	mock := NewMembershipClientMock()
	handler := VersionsEventHandler(logging.Testing(), mock)

	version := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{"name": "v1"},
		},
	}

	handler.OnDelete(version)

	require.Len(t, mock.GetMessages(), 1)
	msg := mock.GetMessages()[0].GetDeletedVersion()
	require.NotNil(t, msg)
	assert.Equal(t, "v1", msg.Name)
}

// --- extractVersionsSpec ---

func TestExtractVersionsSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		obj    *unstructured.Unstructured
		expect map[string]string
	}{
		{
			name: "normal spec",
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"spec": map[string]any{
						"ledger": "v1.0", "payments": "v2.0",
					},
				},
			},
			expect: map[string]string{"ledger": "v1.0", "payments": "v2.0"},
		},
		{
			name: "nil spec",
			obj: &unstructured.Unstructured{
				Object: map[string]any{},
			},
			expect: nil,
		},
		{
			name: "non-string values ignored",
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"spec": map[string]any{
						"ledger": "v1.0",
						"count":  int64(42),
					},
				},
			},
			expect: map[string]string{"ledger": "v1.0"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := extractVersionsSpec(tc.obj)
			assert.Equal(t, tc.expect, result)
		})
	}
}

// --- generateMetadata ---

func TestGenerateMetadata(t *testing.T) {
	t.Parallel()

	listener := newTestListener(&mockK8SClient{}, NewMembershipClientMock(), nil)

	t.Run("with labels and annotations", func(t *testing.T) {
		stack := &generated.Stack{
			AdditionalLabels:      map[string]string{"env": "prod", "team": "infra"},
			AdditionalAnnotations: map[string]string{"note": "important"},
		}
		md := listener.generateMetadata(stack)

		labels := md["labels"].(map[string]any)
		assert.Equal(t, "prod", labels["formance.com/env"])
		assert.Equal(t, "infra", labels["formance.com/team"])

		annotations := md["annotations"].(map[string]any)
		assert.Equal(t, "important", annotations["formance.com/note"])
	})

	t.Run("nil labels and annotations", func(t *testing.T) {
		stack := &generated.Stack{}
		md := listener.generateMetadata(stack)

		labels := md["labels"].(map[string]any)
		assert.Empty(t, labels)

		annotations := md["annotations"].(map[string]any)
		assert.Empty(t, annotations)
	})
}
