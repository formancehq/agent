package helpers

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectorID(t *testing.T) {
	t.Run("String and parse roundtrip", func(t *testing.T) {
		ref := uuid.New()
		cid := NewConnectorID("stripe", ref)

		encoded := cid.String()
		assert.NotEmpty(t, encoded)

		decoded, err := ConnectorIDFromString(encoded)
		require.NoError(t, err)
		assert.Equal(t, "stripe", decoded.Provider)
		assert.Equal(t, ref, decoded.Reference)
	})

	t.Run("Empty connector ID returns empty string", func(t *testing.T) {
		var cid *ConnectorID
		assert.Empty(t, cid.String())

		cid = &ConnectorID{}
		assert.Empty(t, cid.String())
	})

	t.Run("Invalid base64 returns error", func(t *testing.T) {
		_, err := ConnectorIDFromString("not-valid-base64!!!")
		assert.Error(t, err)
	})
}

func TestAccountID(t *testing.T) {
	t.Run("String and parse roundtrip", func(t *testing.T) {
		connectorRef := uuid.New()
		aid := NewAccountID("wise", connectorRef, "external-account-123")

		encoded := aid.String()
		assert.NotEmpty(t, encoded)

		decoded, err := AccountIDFromString(encoded)
		require.NoError(t, err)
		assert.Equal(t, "external-account-123", decoded.Reference)
		assert.Equal(t, "wise", decoded.ConnectorID.Provider)
		assert.Equal(t, connectorRef, decoded.ConnectorID.Reference)
	})

	t.Run("Empty account ID returns empty string", func(t *testing.T) {
		var aid *AccountID
		assert.Empty(t, aid.String())

		aid = &AccountID{}
		assert.Empty(t, aid.String())
	})

	t.Run("Invalid base64 returns error", func(t *testing.T) {
		_, err := AccountIDFromString("not-valid-base64!!!")
		assert.Error(t, err)
	})
}

func TestBuildAccountID(t *testing.T) {
	t.Run("Build from valid connector ID", func(t *testing.T) {
		connectorRef := uuid.New()
		cid := NewConnectorID("modulr", connectorRef)
		connectorIDStr := cid.String()

		accountIDStr, err := BuildAccountID(connectorIDStr, "account-ref-456")
		require.NoError(t, err)
		assert.NotEmpty(t, accountIDStr)

		// Verify we can parse it back
		parsed, err := AccountIDFromString(accountIDStr)
		require.NoError(t, err)
		assert.Equal(t, "account-ref-456", parsed.Reference)
		assert.Equal(t, "modulr", parsed.ConnectorID.Provider)
		assert.Equal(t, connectorRef, parsed.ConnectorID.Reference)
	})

	t.Run("Invalid connector ID returns error", func(t *testing.T) {
		_, err := BuildAccountID("invalid-connector-id", "reference")
		assert.Error(t, err)
	})

	t.Run("MustBuildAccountID panics on invalid input", func(t *testing.T) {
		assert.Panics(t, func() {
			MustBuildAccountID("invalid", "ref")
		})
	})

	t.Run("MustBuildAccountID succeeds with valid input", func(t *testing.T) {
		connectorRef := uuid.New()
		cid := NewConnectorID("stripe", connectorRef)

		assert.NotPanics(t, func() {
			result := MustBuildAccountID(cid.String(), "my-account")
			assert.NotEmpty(t, result)
		})
	})
}

func TestBuildAccountIDFromComponents(t *testing.T) {
	t.Run("Build from components", func(t *testing.T) {
		connectorRef := uuid.New()
		accountIDStr := BuildAccountIDFromComponents("adyen", connectorRef, "merchant-account-789")

		assert.NotEmpty(t, accountIDStr)

		// Verify we can parse it back
		parsed, err := AccountIDFromString(accountIDStr)
		require.NoError(t, err)
		assert.Equal(t, "merchant-account-789", parsed.Reference)
		assert.Equal(t, "adyen", parsed.ConnectorID.Provider)
		assert.Equal(t, connectorRef, parsed.ConnectorID.Reference)
	})
}
