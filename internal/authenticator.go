package internal

import (
	"context"
	"net/http"
	"strconv"

	oidcclient "github.com/zitadel/oidc/v3/pkg/client"
	"golang.org/x/oauth2/clientcredentials"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	metadataID                 = "id"
	metadataBaseUrl            = "baseUrl"
	metadataAdditionalBaseUrls = "additionalBaseUrls"
	metadataProduction         = "production"
	metadataOutdated           = "outdated"
	metadataVersion            = "version"
	metadataCapabilities       = "capabilities"

	capabilityEE         = "EE"
	capabilityModuleList = "MODULE_LIST"
)

type Authenticator interface {
	authenticate(ctx context.Context) (metadata.MD, error)
}
type AuthenticatorFn func(ctx context.Context) (metadata.MD, error)

func (fn AuthenticatorFn) authenticate(ctx context.Context) (metadata.MD, error) {
	return fn(ctx)
}

func TokenAuthenticator(token string) AuthenticatorFn {
	return func(ctx context.Context) (metadata.MD, error) {
		return metadata.New(map[string]string{"token": token}), nil
	}
}

func BearerAuthenticator(issuer, clientID, clientSecret string) AuthenticatorFn {

	return func(ctx context.Context) (metadata.MD, error) {

		discovery, err := oidcclient.Discover(ctx, issuer, http.DefaultClient)
		if err != nil {
			return nil, err
		}

		config := clientcredentials.Config{
			ClientID:     "region_" + clientID,
			ClientSecret: clientSecret,
			TokenURL:     discovery.TokenEndpoint,
		}

		token, err := config.Token(ctx)
		if err != nil {
			return nil, err
		}

		return metadata.New(map[string]string{
			"bearer": token.AccessToken,
		}), nil
	}
}

// MetadataUnaryInterceptor returns a gRPC unary client interceptor that attaches
// authentication and client info metadata to every outgoing unary RPC call.
func MetadataUnaryInterceptor(
	authenticator Authenticator,
	clientInfo ClientInfo,
	modules modules,
	eeModules eeModules,
) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		md, err := authenticator.authenticate(ctx)
		if err != nil {
			return err
		}

		md.Append(metadataID, clientInfo.ID)
		md.Append(metadataBaseUrl, clientInfo.BaseUrl.String())
		md.Append(metadataAdditionalBaseUrls, clientInfo.AdditionalBaseURLs...)
		md.Append(metadataProduction, strconv.FormatBool(clientInfo.Production))
		md.Append(metadataOutdated, strconv.FormatBool(clientInfo.Outdated))
		md.Append(metadataVersion, clientInfo.Version)
		md.Append(metadataCapabilities, capabilityEE, capabilityModuleList)
		md.Append(capabilityModuleList, modules.Singular()...)
		md.Append(capabilityEE, eeModules.Singular()...)

		ctx = metadata.NewOutgoingContext(ctx, md)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
