package internal_test

import (
	"fmt"
	"testing"

	"github.com/formancehq/go-libs/v2/logging"
	"github.com/formancehq/operator/v3/api/formance.com/v1beta1"
	"github.com/formancehq/stack/components/agent/internal"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestDeleteFunc(t *testing.T) {
	t.Parallel()
	reporter := internal.NewMembershipReporterMock()
	resourceInformer := internal.NewStackEventHandler(logging.Testing(), reporter)

	stack := &v1beta1.Stack{
		ObjectMeta: v1.ObjectMeta{
			Name: uuid.NewString(),
		},
	}

	unstructuredStack, err := runtime.DefaultUnstructuredConverter.ToUnstructured(stack)
	if err != nil {
		t.Fatalf("failed to convert stack to unstructured: %v", err)
	}

	resourceInformer.DeleteFunc(&unstructured.Unstructured{
		Object: unstructuredStack,
	})

	events := reporter.GetEvents()
	require.Len(t, events, 1)
	require.Equal(t, "StackDeleted", events[0].Type)
	require.Equal(t, stack.Name, events[0].ClusterName)
}

func TestAddStack(t *testing.T) {
	t.Parallel()
	reporter := internal.NewMembershipReporterMock()
	resourceInformer := internal.NewStackEventHandler(logging.Testing(), reporter)

	stack := &v1beta1.Stack{
		ObjectMeta: v1.ObjectMeta{
			Name: uuid.NewString(),
		},
		Spec: v1beta1.StackSpec{
			Disabled: true,
		},
	}

	unstructuredStack, err := runtime.DefaultUnstructuredConverter.ToUnstructured(stack)
	if err != nil {
		t.Fatalf("failed to convert stack to unstructured: %v", err)
	}
	resourceInformer.AddFunc(&unstructured.Unstructured{
		Object: unstructuredStack,
	})

	events := reporter.GetEvents()
	require.Len(t, events, 1)
	require.Equal(t, "StackStatus", events[0].Type)
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

			reporter := internal.NewMembershipReporterMock()
			resourceInformer := internal.NewStackEventHandler(logging.Testing(), reporter)

			oldStack := &v1beta1.Stack{
				ObjectMeta: v1.ObjectMeta{
					Name: uuid.NewString(),
				},
				Spec: v1beta1.StackSpec{
					Disabled: tc.wasDisabled,
				},
				Status: v1beta1.StackStatus{
					Status: v1beta1.Status{
						Ready: tc.wasReady,
					},
				},
			}

			newStack := oldStack.DeepCopy()
			newStack.Status.Ready = tc.isReady
			newStack.Spec.Disabled = tc.isDisabled

			expectCall := tc.isReady != tc.wasReady || tc.isDisabled != tc.wasDisabled

			unstructuredOldStack, err := runtime.DefaultUnstructuredConverter.ToUnstructured(oldStack)
			if err != nil {
				t.Fatalf("failed to convert old stack to unstructured: %v", err)
			}

			unstructuredNewStack, err := runtime.DefaultUnstructuredConverter.ToUnstructured(newStack)
			if err != nil {
				t.Fatalf("failed to convert new stack to unstructured: %v", err)
			}
			resourceInformer.UpdateFunc(&unstructured.Unstructured{
				Object: unstructuredOldStack,
			}, &unstructured.Unstructured{
				Object: unstructuredNewStack,
			})

			events := reporter.GetEvents()
			if expectCall {
				require.Len(t, events, 1)
				require.Equal(t, "StackStatus", events[0].Type)
			} else {
				require.Empty(t, events)
			}
		})
	}
}
