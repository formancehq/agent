package internal

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestEnsureNotExistBySelector(t *testing.T) {
	test(t, func(ctx context.Context, testConfig *testConfig) {
		k8sClient := NewDefaultK8SClient(testConfig.client)

		moduleCRDs, _, err := RetrieveModuleList(ctx, testConfig.restConfig)
		require.NoError(t, err)

		for _, crd := range moduleCRDs {
			crd := crd
			kind := crd.Spec.Names.Kind
			resource := crd.Status.AcceptedNames.Plural
			version := crd.Spec.Versions[0].Name
			gvk := schema.GroupVersionKind{
				Group:   crd.Spec.Group,
				Version: version,
				Kind:    kind,
			}

			t.Run(fmt.Sprintf("EnsureNotExistBySelector %s", kind), func(t *testing.T) {
				t.Parallel()
				name := uuid.NewString()
				module := unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": gvk.GroupVersion().String(),
						"kind":       kind,
						"metadata": map[string]interface{}{
							"name": name,
							"labels": map[string]interface{}{
								"formance.com/created-by-agent": "true",
								"formance.com/stack":            name,
							},
						},
					},
				}

				require.NoError(t, testConfig.client.Post().Resource(resource).Body(&module).Do(ctx).Error())

				require.NoError(t, k8sClient.EnsureNotExistsBySelector(ctx, resource, stackLabels(module.GetName())))
				require.NoError(t, client.IgnoreNotFound(testConfig.client.Get().Resource(resource).Name(module.GetName()).Do(ctx).Error()))
			})
		}
	})
}
