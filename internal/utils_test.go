package internal

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRestrictStatus(t *testing.T) {

	type testCase struct {
		incomingStatus map[string]interface{}
		expectedStatus map[string]interface{}
		expectError    bool
	}

	conditions := []interface{}{
		map[string]interface{}{
			"type":               uuid.NewString(),
			"reason":             uuid.NewString(),
			"message":            uuid.NewString(),
			"status":             uuid.NewString(),
			"observedGeneration": float64(1),
			"lastTransitionTime": "2024-01-01T00:00:00Z",
		},
		map[string]interface{}{
			"type":               uuid.NewString(),
			"reason":             uuid.NewString(),
			"message":            uuid.NewString(),
			"status":             uuid.NewString(),
			"observedGeneration": float64(2),
			"lastTransitionTime": "2024-01-01T00:00:00Z",
		},
	}

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

			result, err := restrict[status](tc.incomingStatus)
			if tc.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedStatus, result)
		})
	}
}
