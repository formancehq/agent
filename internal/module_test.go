package internal

import (
	"context"
	"testing"

	"github.com/formancehq/go-libs/v2/logging"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/discovery"
)

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
