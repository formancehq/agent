package internal

import (
	"context"
	"reflect"
	"testing"

	"github.com/formancehq/go-libs/v2/logging"
	"github.com/formancehq/operator/v3/api/formance.com/v1beta1"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestMCPIsRegisteredAsModule(t *testing.T) {
	require.NoError(t, v1beta1.AddToScheme(scheme.Scheme))

	gvk := v1beta1.GroupVersion.WithKind("MCP")
	rtype, ok := scheme.Scheme.AllKnownTypes()[gvk]
	require.True(t, ok)

	object := reflect.New(rtype).Interface()
	require.Implements(t, (*v1beta1.Module)(nil), object)
}

func TestGetApiGroupResources(t *testing.T) {
	test(t, func(ctx context.Context, config *testConfig) {
		t.Parallel()
		discovery := discovery.NewDiscoveryClientForConfigOrDie(config.restConfig)
		resource, err := getApiGroupResources(discovery, logging.Testing())
		if err != nil {
			t.Fatalf("failed to get API group resources: %v", err)
		}

		for _, item := range resource {
			require.Equal(t, item.Group.Name, "formance.com", "Expected group name to be 'formance.com'")
		}
	})
}
