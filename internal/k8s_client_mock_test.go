package internal

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
)

// mockK8SClient is a configurable mock for unit-testing error paths.
// Each field is a function that, when set, overrides the default (no-op) behaviour.
type mockK8SClient struct {
	getFn                      func(ctx context.Context, resource, name string) (*unstructured.Unstructured, error)
	applyFn                    func(ctx context.Context, resource string, obj *unstructured.Unstructured, fieldManager string, force bool) (*unstructured.Unstructured, error)
	deleteFn                   func(ctx context.Context, resource, name string) error
	ensureNotExistsFn          func(ctx context.Context, resource, name string) error
	ensureNotExistsBySelectorFn func(ctx context.Context, resource string, selector labels.Selector) error
	listFn                     func(ctx context.Context, resource string, selector labels.Selector) ([]unstructured.Unstructured, error)
}

func (m *mockK8SClient) Get(ctx context.Context, resource, name string) (*unstructured.Unstructured, error) {
	if m.getFn != nil {
		return m.getFn(ctx, resource, name)
	}
	return &unstructured.Unstructured{}, nil
}

func (m *mockK8SClient) Apply(ctx context.Context, resource string, obj *unstructured.Unstructured, fieldManager string, force bool) (*unstructured.Unstructured, error) {
	if m.applyFn != nil {
		return m.applyFn(ctx, resource, obj, fieldManager, force)
	}
	result := obj.DeepCopy()
	result.SetUID("fake-uid")
	return result, nil
}

func (m *mockK8SClient) Delete(ctx context.Context, resource, name string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, resource, name)
	}
	return nil
}

func (m *mockK8SClient) EnsureNotExists(ctx context.Context, resource, name string) error {
	if m.ensureNotExistsFn != nil {
		return m.ensureNotExistsFn(ctx, resource, name)
	}
	return nil
}

func (m *mockK8SClient) EnsureNotExistsBySelector(ctx context.Context, resource string, selector labels.Selector) error {
	if m.ensureNotExistsBySelectorFn != nil {
		return m.ensureNotExistsBySelectorFn(ctx, resource, selector)
	}
	return nil
}

func (m *mockK8SClient) List(ctx context.Context, resource string, selector labels.Selector) ([]unstructured.Unstructured, error) {
	if m.listFn != nil {
		return m.listFn(ctx, resource, selector)
	}
	return nil, nil
}
