package grpcclient

import (
	"context"
	"fmt"
	"reflect"

	"github.com/formancehq/stack/components/agent/internal/generated"
	"github.com/formancehq/stack/components/agent/pkg/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("cmd.formance.grpc")

//go:generate mockgen -source=connection_adapter.go -destination=connection_generated.go -package grpcserver . Connection
type Connection interface {
	CloseSend() error
	Send(*generated.Message) error
	Recv() (*generated.Order, error)
}

//go:generate mockgen -source=connection_adapter.go -destination=connection_generated.go -package grpcserver . ConnectionAdapter
type ConnectionAdapter interface {
	CloseSend(context.Context) error

	Send(context.Context, *generated.Message) error
	Recv(context.Context) (*generated.Order, error)
}

type DefaultConnection struct {
	Connection
}

var _ ConnectionAdapter = (*DefaultConnection)(nil)

func (c *DefaultConnection) Send(_ context.Context, msg *generated.Message) error {
	return c.Connection.Send(msg)
}

func (c *DefaultConnection) Recv(_ context.Context) (*generated.Order, error) {
	return c.Connection.Recv()
}

func (c *DefaultConnection) CloseSend(_ context.Context) error {
	return c.Connection.CloseSend()
}

func NewDefaultConnection(conn Connection) *DefaultConnection {
	return &DefaultConnection{
		Connection: conn,
	}
}

type ConnectionWithTrace struct {
	Debug bool
	Connection
}

func NewConnectionWithTrace(conn Connection, debug bool) *ConnectionWithTrace {
	return &ConnectionWithTrace{
		Connection: conn,
		Debug:      debug,
	}
}

func (c *ConnectionWithTrace) Send(ctx context.Context, msg *generated.Message) error {
	return tracing.TraceError(ctx, tracer, "Send", func(ctx context.Context) error {
		span := trace.SpanFromContext(ctx)
		name := reflect.TypeOf(msg.Message).Elem().Name()
		span.SetAttributes(attribute.String("grpc.message.type", name))
		if c.Debug {
			span.SetAttributes(attribute.String("grpc.message.raw", fmt.Sprintf("%v", msg.String())))
		}

		InjectOtelCtxInMessage(ctx, msg)
		return c.Connection.Send(msg)
	})
}

func (c *ConnectionWithTrace) Recv(ctx context.Context) (*generated.Order, error) {
	msg, err := c.Connection.Recv()
	if err != nil {
		return nil, err
	}
	ctx = ExtractOtelCtxFromMessage(ctx, msg)
	return tracing.Trace(ctx, tracer, "Recv", func(ctx context.Context) (*generated.Order, error) {
		span := trace.SpanFromContext(ctx)
		name := reflect.TypeOf(msg.Message).Elem().Name()
		span.SetAttributes(attribute.String("grpc.message.type", name))

		if c.Debug {
			span.SetAttributes(attribute.String("grpc.message.raw", fmt.Sprintf("%v", msg.String())))
		}
		return msg, err
	})
}

func (c *ConnectionWithTrace) CloseSend(ctx context.Context) error {
	return tracing.TraceError(ctx, tracer, "CloseSend", func(ctx context.Context) error {
		return c.Connection.CloseSend()
	})
}
