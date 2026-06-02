package internal

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	osRuntime "runtime"
	"testing"
	"time"

	v1apis "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"github.com/formancehq/go-libs/v2/logging"
	"github.com/formancehq/stack/components/agent/internal/generated"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var formanceGV = schema.GroupVersion{Group: "formance.com", Version: "v1beta1"}

type testConfig struct {
	restConfig *rest.Config
	mapper     meta.RESTMapper
	client     *rest.RESTClient
}

func test(t *testing.T, fn func(context.Context, *testConfig)) {
	_, filename, _, _ := osRuntime.Caller(0)
	apiServer := envtest.APIServer{}
	apiServer.Configure().
		Set("service-cluster-ip-range", "10.0.0.0/20")

	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join(filepath.Dir(filename), "..", "dist", "operator",
				"config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
		ControlPlane: envtest.ControlPlane{
			APIServer: &apiServer,
		},
	}

	restConfig, err := testEnv.Start()

	require.NoError(t, err)

	restConfig.GroupVersion = &formanceGV
	restConfig.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	restConfig.APIPath = "/apis"

	k8sClient, err := rest.RESTClientFor(restConfig)
	require.NoError(t, err)

	mapper, err := CreateRestMapper(restConfig, logging.Testing())
	require.NoError(t, err)

	t.Cleanup(
		func() {
			require.NoError(t, testEnv.Stop())
		},
	)
	fn(logging.TestingContext(), &testConfig{
		restConfig: restConfig,
		mapper:     mapper,
		client:     k8sClient,
	})
}
func TestDeleteModule(t *testing.T) {

	type testCase struct {
		name       string
		withLabels bool
	}

	testCases := []testCase{
		{
			name:       "with labels",
			withLabels: true,
		},
		{
			name:       "without labels",
			withLabels: false,
		},
	}
	test(t, func(ctx context.Context, testConfig *testConfig) {
		t.Parallel()

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				stackName := uuid.NewString()
				recon := &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": formanceGV.String(),
						"kind":       "Reconciliation",
						"metadata": map[string]interface{}{
							"name": uuid.NewString(),
						},
					},
				}
				if tc.withLabels {
					recon.SetLabels(map[string]string{
						"formance.com/created-by-agent": "true",
						"formance.com/stack":            stackName,
					})
				}

				gvk := formanceGV.WithKind("Reconciliation")
				resources, err := testConfig.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
				require.NoError(t, err)

				require.NoError(t, testConfig.client.Post().Resource(resources.Resource.Resource).Body(recon).Do(ctx).Error())
				orders := NewMembershipClientMock()

				membershipListener := NewMembershipListener(NewDefaultK8SClient(testConfig.client), ClientInfo{}, testConfig.mapper, orders, []v1apis.CustomResourceDefinition{})

				if tc.withLabels {
					require.NoError(t, membershipListener.deleteModule(ctx, logging.Testing(), resources.Resource.Resource, stackName))
					require.Error(t, testConfig.client.Get().Resource(resources.Resource.Resource).Name(recon.GetName()).Do(ctx).Error())
				}

				if !tc.withLabels {
					require.NoError(t, testConfig.client.Get().Resource(resources.Resource.Resource).Name(recon.GetName()).Do(ctx).Error())
				}
			})
		}
	})
}

func TestRetrieveModuleList(t *testing.T) {
	t.Parallel()
	test(t, func(ctx context.Context, testConfig *testConfig) {
		modules, eeModules, err := RetrieveModuleList(ctx, testConfig.restConfig)
		require.NoError(t, err)
		require.NotEmpty(t, modules)
		require.NotEmpty(t, eeModules)
		for _, module := range eeModules {
			require.Contains(t, modules, module)
		}
	})
}
func TestSyncAuthClients(t *testing.T) {
	letter := []rune("abcdefghijklmnopqrstuvwxyz")
	randStr := func(i int) string {
		b := make([]rune, i)
		for i := range b {
			b[i] = letter[rand.Intn(len(letter))]
		}
		return string(b)
	}

	newStaticClient := func(stackName string) *unstructured.Unstructured {
		return &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": formanceGV.String(),
				"kind":       "AuthClient",
				"metadata": map[string]interface{}{
					"name": uuid.NewString(),
					"labels": map[string]interface{}{
						"formance.com/created-by-agent": "true",
						"formance.com/stack":            stackName,
					},
				},
			},
		}
	}

	newGeneratedClient := func() *generated.AuthClient {
		return &generated.AuthClient{
			Id:     randStr(4),
			Public: true,
		}
	}
	test(t, func(ctx context.Context, tc *testConfig) {
		t.Parallel()
		listener := NewMembershipListener(NewDefaultK8SClient(tc.client), ClientInfo{}, tc.mapper, NewMembershipClientMock(), []v1apis.CustomResourceDefinition{})

		stackName := uuid.NewString() + "-" + randStr(4)
		stackuid := uuid.NewString()

		authClientsToRemove := []*unstructured.Unstructured{
			newStaticClient(stackName),
			newStaticClient(stackName),
			newStaticClient(stackName),
		}

		clients := []*generated.AuthClient{
			newGeneratedClient(),
			newGeneratedClient(),
		}

		stack := &unstructured.Unstructured{}
		stack.SetName(stackName)
		stack.SetUID(types.UID(stackuid))

		for _, client := range authClientsToRemove {
			require.NoError(t, tc.client.Post().Resource("AuthClients").Body(client).Do(ctx).Error())
		}

		listener.syncAuthClients(ctx, map[string]any{}, stack, clients)

		clientsList := &unstructured.UnstructuredList{}
		require.Eventually(t, func() bool {
			err := tc.client.Get().Resource("AuthClients").
				VersionedParams(&v1.ListOptions{}, v1.ParameterCodec).
				Do(ctx).Into(clientsList)
			require.NoError(t, err)
			return len(clientsList.Items) == len(clients)
		}, 5*time.Second, 500*time.Millisecond)
	})
}

func TestSyncStargate(t *testing.T) {
	type testCase struct {
		enabled bool
	}
	letter := []rune("abcdefghijklmnopqrstuvwxyz")
	randStr := func(i int) string {
		b := make([]rune, i)
		for i := range b {
			b[i] = letter[rand.Intn(len(letter))]
		}
		return string(b)
	}

	for _, tcase := range []testCase{
		{
			enabled: true,
		},
		{},
	} {
		t.Run(fmt.Sprintf("%s enabled=%t", t.Name(), tcase.enabled), func(t *testing.T) {
			test(t, func(ctx context.Context, tc *testConfig) {
				t.Parallel()
				listener := NewMembershipListener(NewDefaultK8SClient(tc.client), ClientInfo{}, tc.mapper, NewMembershipClientMock(), []v1apis.CustomResourceDefinition{})

				stackName := uuid.NewString() + "-" + randStr(4)
				stackuid := uuid.NewString()
				stack := &unstructured.Unstructured{}
				stack.SetName(stackName)
				stack.SetUID(types.UID(stackuid))

				stargateConfig := &generated.StargateConfig{
					Enabled: true,
				}

				// Create a stargate module
				listener.syncStargate(ctx, map[string]any{}, stack, &generated.Stack{
					AuthConfig:     &generated.AuthConfig{},
					StargateConfig: stargateConfig,
				})
				fStargate := &unstructured.Unstructured{}
				require.Eventually(t, func() bool {
					return tc.client.Get().Resource("Stargates").Name(stackName).Do(ctx).Into(fStargate) == nil
				}, 5*time.Second, 500*time.Millisecond)

				fStargateRV := fStargate.GetResourceVersion()

				// Sync depending of the config
				listener.syncStargate(ctx, map[string]any{}, stack, &generated.Stack{
					StargateConfig: &generated.StargateConfig{
						Enabled: tcase.enabled,
					},
					AuthConfig: &generated.AuthConfig{
						ClientId: uuid.NewString(),
					},
				})

				if !tcase.enabled {
					require.Eventually(t, func() bool {
						err := tc.client.Get().Resource("Stargates").Name(stackName).Do(ctx).Error()
						return err != nil && apierrors.IsNotFound(err)
					}, 5*time.Second, 500*time.Millisecond)
					return
				} else {
					require.Eventually(t, func() bool {
						stargate := &unstructured.Unstructured{}
						err := tc.client.Get().Resource("Stargates").Name(stackName).Do(ctx).Into(stargate)
						return err == nil && stargate.GetResourceVersion() != fStargateRV
					}, 5*time.Second, 500*time.Millisecond)
				}
			})
		})

	}

}

func TestDeleteStackNotExisting(t *testing.T) {
	test(t, func(ctx context.Context, tc *testConfig) {
		t.Parallel()
		mock := NewMembershipClientMock()
		listener := NewMembershipListener(NewDefaultK8SClient(tc.client), ClientInfo{}, tc.mapper, mock, []v1apis.CustomResourceDefinition{})
		listener.deleteStack(ctx, &generated.DeletedStack{
			ClusterName: "non-existing-stack",
		})

		messages := mock.GetMessages()
		require.Len(t, messages, 1)

		message, ok := messages[0].Message.(*generated.Message_StackDeleted)
		require.True(t, ok)

		require.Equal(t, "non-existing-stack", message.StackDeleted.ClusterName)
	})
}
