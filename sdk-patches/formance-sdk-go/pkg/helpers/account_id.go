// Package helpers provides utility functions for working with Formance API entities.
// This file is NOT auto-generated and can be freely modified.
package helpers

import (
	"encoding/base64"
	"fmt"

	"github.com/gibson042/canonicaljson-go"
	"github.com/google/uuid"
)

// ConnectorID represents a Formance Payments connector identifier.
type ConnectorID struct {
	Reference uuid.UUID `json:"Reference"`
	Provider  string    `json:"Provider"`
}

// String returns the string representation of a ConnectorID.
// The format is base64 URL encoded canonical JSON.
func (cid *ConnectorID) String() string {
	if cid == nil || cid.Reference == uuid.Nil {
		return ""
	}

	data, err := canonicaljson.Marshal(cid)
	if err != nil {
		panic(err)
	}

	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(data)
}

// ConnectorIDFromString parses a connector ID string and returns a ConnectorID.
func ConnectorIDFromString(value string) (ConnectorID, error) {
	data, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(value)
	if err != nil {
		return ConnectorID{}, fmt.Errorf("failed to decode connector ID: %w", err)
	}

	var ret ConnectorID
	err = canonicaljson.Unmarshal(data, &ret)
	if err != nil {
		return ConnectorID{}, fmt.Errorf("failed to unmarshal connector ID: %w", err)
	}

	return ret, nil
}

// MustConnectorIDFromString parses a connector ID string and panics on error.
func MustConnectorIDFromString(value string) ConnectorID {
	id, err := ConnectorIDFromString(value)
	if err != nil {
		panic(err)
	}
	return id
}

// AccountID represents a Formance Payments account identifier.
type AccountID struct {
	Reference   string      `json:"Reference"`
	ConnectorID ConnectorID `json:"ConnectorID"`
}

// String returns the string representation of an AccountID.
// The format is base64 URL encoded canonical JSON.
func (aid *AccountID) String() string {
	if aid == nil || aid.Reference == "" {
		return ""
	}

	data, err := canonicaljson.Marshal(aid)
	if err != nil {
		panic(err)
	}

	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(data)
}

// AccountIDFromString parses an account ID string and returns an AccountID.
func AccountIDFromString(value string) (AccountID, error) {
	data, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(value)
	if err != nil {
		return AccountID{}, fmt.Errorf("failed to decode account ID: %w", err)
	}

	var ret AccountID
	err = canonicaljson.Unmarshal(data, &ret)
	if err != nil {
		return AccountID{}, fmt.Errorf("failed to unmarshal account ID: %w", err)
	}

	return ret, nil
}

// MustAccountIDFromString parses an account ID string and panics on error.
func MustAccountIDFromString(value string) AccountID {
	id, err := AccountIDFromString(value)
	if err != nil {
		panic(err)
	}
	return id
}

// BuildAccountID creates an account ID string from a connector ID string and a reference.
// The connectorID should be a valid base64 encoded connector ID.
// The reference is the external account identifier (e.g., account number, IBAN, etc.).
func BuildAccountID(connectorID string, reference string) (string, error) {
	cid, err := ConnectorIDFromString(connectorID)
	if err != nil {
		return "", fmt.Errorf("invalid connector ID: %w", err)
	}

	aid := AccountID{
		Reference:   reference,
		ConnectorID: cid,
	}

	return aid.String(), nil
}

// MustBuildAccountID creates an account ID string from a connector ID and reference.
// It panics if the connector ID is invalid.
func MustBuildAccountID(connectorID string, reference string) string {
	id, err := BuildAccountID(connectorID, reference)
	if err != nil {
		panic(err)
	}
	return id
}

// NewAccountID creates a new AccountID from individual components.
// This is useful when you have the raw connector components rather than an encoded connector ID.
func NewAccountID(provider string, connectorReference uuid.UUID, accountReference string) *AccountID {
	return &AccountID{
		Reference: accountReference,
		ConnectorID: ConnectorID{
			Reference: connectorReference,
			Provider:  provider,
		},
	}
}

// NewConnectorID creates a new ConnectorID from individual components.
func NewConnectorID(provider string, reference uuid.UUID) *ConnectorID {
	return &ConnectorID{
		Reference: reference,
		Provider:  provider,
	}
}

// BuildAccountIDFromComponents creates an account ID string from raw components.
// provider: the payment provider name (e.g., "stripe", "wise", "modulr")
// connectorReference: the UUID reference of the connector
// accountReference: the external account identifier
func BuildAccountIDFromComponents(provider string, connectorReference uuid.UUID, accountReference string) string {
	aid := NewAccountID(provider, connectorReference, accountReference)
	return aid.String()
}
