package internal

import (
	"context"
	"io"
	"strconv"
	"time"

	"github.com/formancehq/go-libs/v2/logging"
	"github.com/formancehq/stack/components/agent/internal/generated"
	"github.com/formancehq/stack/components/agent/internal/grpcclient"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	metadataID           = "id"
	metadataBaseUrl      = "baseUrl"
	metadataProduction   = "production"
	metadataOutdated     = "outdated"
	metadataVersion      = "version"
	metadataCapabilities = "capabilities"

	capabilityEE         = "EE"
	capabilityModuleList = "MODULE_LIST"
)

type membershipClient struct {
	clientInfo ClientInfo
	stopChan   chan chan error
	stopped    chan struct{}

	joinContext context.Context
	joinCancel  func()

	authenticator Authenticator

	orders chan *generated.Order
	opts   []grpc.DialOption

	address string

	messages chan *generated.Message
}

func (c *membershipClient) connectMetadata(ctx context.Context, modules []string, eeModules []string) (metadata.MD, error) {

	md, err := c.authenticator.authenticate(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "authenticating client")
	}

	md.Append(metadataID, c.clientInfo.ID)
	md.Append(metadataBaseUrl, c.clientInfo.BaseUrl.String())
	md.Append(metadataProduction, strconv.FormatBool(c.clientInfo.Production))
	md.Append(metadataOutdated, strconv.FormatBool(c.clientInfo.Outdated))
	md.Append(metadataVersion, c.clientInfo.Version)
	md.Append(metadataCapabilities, capabilityEE, capabilityModuleList)
	md.Append(capabilityModuleList, modules...)
	md.Append(capabilityEE, eeModules...)
	return md, nil
}

func LoggingClientStreamInterceptor(l logging.Logger) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		logging.FromContext(ctx).
			Infof("Starting stream")
		return streamer(logging.ContextWithLogger(ctx, l), desc, cc, method, opts...)
	}
}

func (c *membershipClient) connect(ctx context.Context, modules []string, eeModules []string) (generated.Server_JoinClient, error) {
	logging.FromContext(ctx).WithFields(map[string]any{
		"id": c.clientInfo.ID,
	}).Infof("Establish connection to server")
	c.joinContext, c.joinCancel = context.WithCancel(ctx)

	opts := append(c.opts,
		grpc.WithChainStreamInterceptor(
			LoggingClientStreamInterceptor(logging.FromContext(ctx)),
		),
	)
	conn, err := grpc.NewClient(c.address, opts...)
	if err != nil {
		return nil, err
	}

	serverClient := generated.NewServerClient(conn)

	md, err := c.connectMetadata(ctx, modules, eeModules)
	if err != nil {
		return nil, err
	}
	connectContext := metadata.NewOutgoingContext(c.joinContext, md)
	joinClient, err := serverClient.Join(connectContext)
	if err != nil {
		return nil, err
	}

	return joinClient, nil
}

func (c *membershipClient) Send(message *generated.Message) error {
	select {
	case <-c.stopped:
		return errors.New("stopped")
	case c.messages <- message:
		return nil
	}
}

func (c *membershipClient) sendPong(ctx context.Context, client grpcclient.ConnectionAdapter) {
	if err := client.Send(ctx, &generated.Message{
		Message: &generated.Message_Pong{
			Pong: &generated.Pong{},
		},
	}); err != nil {
		logging.FromContext(ctx).Errorf("Unable to send pong to server: %s", err)
		if errors.Is(err, io.EOF) {
			panic(err)
		}
	}
}

func (c *membershipClient) Start(ctx context.Context, client grpcclient.ConnectionAdapter) error {

	var (
		errCh = make(chan error, 1)
	)
	go func() {
		for {
			msg, err := client.Recv(ctx)
			if err != nil {
				if err == io.EOF {
					select {
					case <-c.stopped:
					default:
						errCh <- err
					}
					return
				}
				errCh <- err
				return
			}

			if msg.GetPing() != nil {
				c.sendPong(ctx, client)
				continue
			}

			select {
			case c.orders <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		for {
			select {
			case <-time.After(5 * time.Second):
				c.sendPong(ctx, client)
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ch := <-c.stopChan:
			close(c.stopped)
			if err := client.CloseSend(ctx); err != nil {
				ch <- err
				//nolint:nilerr
				return nil
			}
			c.joinCancel()

			// Drain messages
			for {
				_, err := client.Recv(ctx)
				if err != nil {
					break
				}
			}

			ch <- nil
			return nil
		case msg := <-c.messages:
			if err := client.Send(ctx, msg); err != nil {
				panic(err)
			}
			<-time.After(50 * time.Millisecond)
		case err := <-errCh:
			logging.FromContext(ctx).Errorf("Stream closed with error: %s", err)
			return err
		}
	}
}

func (c *membershipClient) Stop(ctx context.Context) error {
	ch := make(chan error)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case c.stopChan <- ch:
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-ch:
			return err
		}
	}
}

func (c *membershipClient) Orders() chan *generated.Order {
	return c.orders
}

func NewMembershipClient(authenticator Authenticator, clientInfo ClientInfo, address string, opts ...grpc.DialOption) *membershipClient {
	return &membershipClient{
		stopChan:      make(chan chan error),
		authenticator: authenticator,
		clientInfo:    clientInfo,
		opts:          opts,
		address:       address,
		orders:        make(chan *generated.Order),
		messages:      make(chan *generated.Message),
		stopped:       make(chan struct{}),
	}
}
