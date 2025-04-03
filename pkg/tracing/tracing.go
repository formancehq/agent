package tracing

import (
	"context"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func TraceTuple[K, V any](ctx context.Context, tracer trace.Tracer, name string, fn func(ctx context.Context) (K, V, error), opts ...trace.SpanStartOption) (K, V, error) {
	ctx, trace := tracer.Start(ctx, name, opts...)
	defer trace.End()

	var k K
	var v V
	k, v, err := fn(ctx)
	if err != nil {
		trace.RecordError(err)
		trace.SetStatus(codes.Error, err.Error())
		return k, v, err
	}

	return k, v, nil
}

func Trace[RET any](ctx context.Context, tracer trace.Tracer, name string, fn func(ctx context.Context) (RET, error), opts ...trace.SpanStartOption) (RET, error) {
	ctx, trace := tracer.Start(ctx, name, opts...)
	defer trace.End()

	var zeroRet RET
	ret, err := fn(ctx)
	if err != nil {
		trace.RecordError(err)
		trace.SetStatus(codes.Error, err.Error())
		return zeroRet, err
	}

	return ret, nil
}

func TraceError(ctx context.Context, tracer trace.Tracer, name string, fn func(ctx context.Context) error, opts ...trace.SpanStartOption) error {
	ctx, trace := tracer.Start(ctx, name, opts...)
	defer trace.End()

	if err := fn(ctx); err != nil {
		trace.RecordError(err)
		trace.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}
