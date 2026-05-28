package internal_test

import (
	"testing"

	"github.com/formancehq/go-libs/v2/logging"
	"github.com/formancehq/stack/components/agent/internal"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestModuleAddFunc(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	membershipClientMock := internal.NewMockMembershipClient(ctrl)
	resourceInformer := internal.NewModuleEventHandler(logging.Testing(), membershipClientMock)

	module := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": uuid.NewString(),
			},
			"status": map[string]interface{}{
				"ready": false,
			},
		},
	}

	membershipClientMock.EXPECT().Send(gomock.Any())
	resourceInformer.AddFunc(module)
	require.True(t, ctrl.Satisfied())

}

func TestModuleDelete(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	membershipClientMock := internal.NewMockMembershipClient(ctrl)
	resourceInformer := internal.NewModuleEventHandler(logging.Testing(), membershipClientMock)

	module := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": uuid.NewString(),
			},
			"status": map[string]interface{}{
				"ready": false,
			},
		},
	}

	membershipClientMock.EXPECT().Send(gomock.Any())
	resourceInformer.DeleteFunc(module)
	require.True(t, ctrl.Satisfied())
}

func TestModuleUpdateStatusNil(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	membershipClientMock := internal.NewMockMembershipClient(ctrl)
	resourceInformer := internal.NewModuleEventHandler(logging.Testing(), membershipClientMock)

	oldModule := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": uuid.NewString(),
			},
		},
	}

	newModule := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": uuid.NewString(),
			},
		},
	}

	resourceInformer.UpdateFunc(oldModule, newModule)
	require.True(t, ctrl.Satisfied())

}
func TestModuleUpdateStatusChanged(t *testing.T) {

	type testCase struct {
		isReady  bool
		wasReady bool
	}

	testCases := []testCase{}
	for _, a := range []bool{true, false} {
		for _, b := range []bool{true, false} {
			testCases = append(testCases, testCase{a, b})
		}
	}

	for _, tc := range testCases {
		tc := tc
		t.Run("test", func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			membershipClientMock := internal.NewMockMembershipClient(ctrl)
			resourceInformer := internal.NewModuleEventHandler(logging.Testing(), membershipClientMock)

			oldModule := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": uuid.NewString(),
					},
					"status": map[string]interface{}{
						"ready": tc.wasReady,
					},
				},
			}

			newModule := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": uuid.NewString(),
					},
					"status": map[string]interface{}{
						"ready": tc.isReady,
					},
				},
			}
			if tc.isReady != tc.wasReady {
				membershipClientMock.EXPECT().Send(gomock.Any())
			}
			resourceInformer.UpdateFunc(oldModule, newModule)
			require.True(t, ctrl.Satisfied())
		})
	}

}
