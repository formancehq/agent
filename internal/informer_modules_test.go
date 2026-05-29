package internal_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/formancehq/go-libs/v2/collectionutils"
	"github.com/formancehq/go-libs/v2/logging"
	"github.com/formancehq/operator/v3/api/formance.com/v1beta1"
	"github.com/formancehq/stack/components/agent/internal"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRestrictModuleStatus(t *testing.T) {

	type testCase struct {
		incomingStatus map[string]interface{}
		expectedStatus map[string]interface{}
		expectError    bool
	}
	conditions := func() []interface{} {
		conditions := []v1beta1.Condition{}

		var count int64 = 0
		newCondition := func() v1beta1.Condition {
			count++
			return v1beta1.Condition{
				Type:               uuid.NewString(),
				Reason:             uuid.NewString(),
				Message:            uuid.NewString(),
				Status:             v1.ConditionStatus(uuid.NewString()),
				ObservedGeneration: count,
				LastTransitionTime: v1.Time{},
			}
		}

		conditions = append(conditions, newCondition())
		conditions = append(conditions, newCondition())
		return collectionutils.Map(conditions, func(c v1beta1.Condition) interface{} {
			b, err := json.Marshal(c)
			if err != nil {
				t.Fatal(err)
			}
			var m map[string]interface{}
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatal(err)
			}
			return m
		})
	}()
	testCases := []testCase{
		{
			incomingStatus: map[string]interface{}{},
			expectedStatus: map[string]interface{}{},
			expectError:    true,
		},
		{
			incomingStatus: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			expectedStatus: map[string]interface{}{
				"ready": false,
			},
		},
		{
			incomingStatus: map[string]interface{}{
				"info":  "some info",
				"ready": true,
			},
			expectedStatus: map[string]interface{}{
				"info":  "some info",
				"ready": true,
			},
		},
		{
			incomingStatus: map[string]interface{}{
				"conditions": conditions,
			},

			expectedStatus: map[string]interface{}{
				"ready":      false,
				"conditions": conditions,
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run("test", func(t *testing.T) {
			t.Parallel()

			status, err := internal.Restrict[v1beta1.Status](tc.incomingStatus)
			if tc.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedStatus, status)
		})
	}
}

func TestModuleAddFunc(t *testing.T) {
	t.Parallel()
	reporter := internal.NewMembershipReporterMock()
	resourceInformer := internal.NewModuleEventHandler(context.Background(), logging.Testing(), reporter)

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

	resourceInformer.AddFunc(module)

	events := reporter.GetEvents()
	require.Len(t, events, 1)
	require.Equal(t, "ModuleStatus", events[0].Type)
}

func TestModuleDelete(t *testing.T) {
	t.Parallel()
	reporter := internal.NewMembershipReporterMock()
	resourceInformer := internal.NewModuleEventHandler(context.Background(), logging.Testing(), reporter)

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

	resourceInformer.DeleteFunc(module)

	events := reporter.GetEvents()
	require.Len(t, events, 1)
	require.Equal(t, "ModuleDeleted", events[0].Type)
}

func TestModuleUpdateStatusNil(t *testing.T) {
	t.Parallel()
	reporter := internal.NewMembershipReporterMock()
	resourceInformer := internal.NewModuleEventHandler(context.Background(), logging.Testing(), reporter)

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

	events := reporter.GetEvents()
	require.Empty(t, events)
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
			reporter := internal.NewMembershipReporterMock()
			resourceInformer := internal.NewModuleEventHandler(context.Background(), logging.Testing(), reporter)

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

			resourceInformer.UpdateFunc(oldModule, newModule)

			events := reporter.GetEvents()
			if tc.isReady != tc.wasReady {
				require.Len(t, events, 1)
				require.Equal(t, "ModuleStatus", events[0].Type)
			} else {
				require.Empty(t, events)
			}
		})
	}

}
