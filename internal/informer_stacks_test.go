package internal_test

import (
	"fmt"
	"testing"

	"github.com/formancehq/go-libs/v2/logging"
	"github.com/formancehq/stack/components/agent/internal"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func newUnstructuredStack(name string, ready bool, disabled bool) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "formance.com/v1beta1",
			"kind":       "Stack",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": map[string]interface{}{
				"disabled": disabled,
			},
			"status": map[string]interface{}{
				"ready": ready,
			},
		},
	}
}

func TestDeleteFunc(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	membershipClientMock := internal.NewMockMembershipClient(ctrl)
	resourceInformer := internal.NewStackEventHandler(logging.Testing(), membershipClientMock)

	stack := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "formance.com/v1beta1",
			"kind":       "Stack",
			"metadata": map[string]interface{}{
				"name": uuid.NewString(),
			},
		},
	}

	membershipClientMock.EXPECT().Send(gomock.Any())
	resourceInformer.DeleteFunc(stack)

	require.True(t, ctrl.Satisfied())
}

func TestAddStack(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	membershipClientMock := internal.NewMockMembershipClient(ctrl)
	resourceInformer := internal.NewStackEventHandler(logging.Testing(), membershipClientMock)

	stack := newUnstructuredStack(uuid.NewString(), false, true)

	membershipClientMock.EXPECT().Send(gomock.Any())
	resourceInformer.AddFunc(stack)

	require.True(t, ctrl.Satisfied())
}

// We are watching .Status and .Spec fields of the stack resource.
// Simulating a change in the status or spec of the stack resource should trigger a call to the membership client.
func TestUpdateStatus(t *testing.T) {
	type testCase struct {
		isReady    bool
		isDisabled bool

		wasReady    bool
		wasDisabled bool
	}
	testCases := []testCase{}

	for _, b := range []bool{true, false} {
		for _, c := range []bool{true, false} {
			for _, d := range []bool{true, false} {
				for _, e := range []bool{true, false} {
					testCases = append(testCases, testCase{
						isReady:    b,
						isDisabled: c,

						wasReady:    d,
						wasDisabled: e,
					})
				}
			}
		}
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(fmt.Sprintf("isReady: %t isDisabled: %t wasReady: %t wasDisabled: %t", tc.isReady, tc.isDisabled, tc.wasReady, tc.wasDisabled), func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			membershipClientMock := internal.NewMockMembershipClient(ctrl)
			resourceInformer := internal.NewStackEventHandler(logging.Testing(), membershipClientMock)

			name := uuid.NewString()
			oldStack := newUnstructuredStack(name, tc.wasReady, tc.wasDisabled)
			newStack := newUnstructuredStack(name, tc.isReady, tc.isDisabled)

			if tc.isReady != tc.wasReady || tc.isDisabled != tc.wasDisabled {
				membershipClientMock.EXPECT().Send(gomock.Any())
			}

			resourceInformer.UpdateFunc(oldStack, newStack)

			require.True(t, ctrl.Satisfied())
		})
	}
}
