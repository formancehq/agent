# Formance SDK Go - Account ID Helpers

This patch adds helper functions to build and parse Formance Payments account IDs
and connector IDs to the [formance-sdk-go](https://github.com/formancehq/formance-sdk-go) SDK.

## Files to Add

Copy the following files to your local clone of `formance-sdk-go`:

```
pkg/helpers/account_id.go      - Main helper functions
pkg/helpers/account_id_test.go - Unit tests
```

## Dependencies to Add

Run the following commands to add the required dependencies:

```bash
go get github.com/gibson042/canonicaljson-go github.com/google/uuid
go mod tidy
```

Or apply the patch:

```bash
git apply go.mod.patch
```

## Usage

### Building an Account ID from Connector ID and Reference

```go
import "github.com/formancehq/formance-sdk-go/v3/pkg/helpers"

// If you have an encoded connector ID (from the API)
connectorID := "eyJQcm92aWRlciI6InN0cmlwZSIsIlJlZmVyZW5jZSI6IjEyMzQ1Njc4LTEyMzQtMTIzNC0xMjM0LTEyMzQ1Njc4OTAxMiJ9"
accountReference := "acct_1234567890"

accountID, err := helpers.BuildAccountID(connectorID, accountReference)
if err != nil {
    log.Fatal(err)
}
fmt.Println(accountID)

// Or use the Must version (panics on error)
accountID = helpers.MustBuildAccountID(connectorID, accountReference)
```

### Building an Account ID from Components

```go
import (
    "github.com/formancehq/formance-sdk-go/v3/pkg/helpers"
    "github.com/google/uuid"
)

provider := "stripe"
connectorRef := uuid.MustParse("12345678-1234-1234-1234-123456789012")
accountRef := "acct_1234567890"

accountID := helpers.BuildAccountIDFromComponents(provider, connectorRef, accountRef)
fmt.Println(accountID)
```

### Parsing an Account ID

```go
import "github.com/formancehq/formance-sdk-go/v3/pkg/helpers"

accountIDStr := "eyJDb25uZWN0b3JJRCI6eyJQcm92aWRlciI6InN0cmlwZSIsIlJlZmVyZW5jZSI6IjEyMzQ..."

accountID, err := helpers.AccountIDFromString(accountIDStr)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Provider: %s\n", accountID.ConnectorID.Provider)
fmt.Printf("Connector Reference: %s\n", accountID.ConnectorID.Reference)
fmt.Printf("Account Reference: %s\n", accountID.Reference)
```

### Working with Connector IDs

```go
import "github.com/formancehq/formance-sdk-go/v3/pkg/helpers"

// Create a new connector ID
cid := helpers.NewConnectorID("stripe", uuid.New())
encodedCID := cid.String()

// Parse a connector ID
parsedCID, err := helpers.ConnectorIDFromString(encodedCID)
```

## Encoding Format

The Account ID and Connector ID use the same encoding format as the Formance Payments service:

- Canonical JSON serialization (using [canonicaljson-go](https://github.com/gibson042/canonicaljson-go))
- Base64 URL encoding without padding

This ensures compatibility with IDs returned by the Formance API.

## Running Tests

```bash
go test ./pkg/helpers/... -v
```
