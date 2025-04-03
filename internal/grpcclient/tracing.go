package grpcclient

import (
	"context"
	"encoding/json"

	"github.com/formancehq/go-libs/v2/logging"
	"github.com/formancehq/stack/components/agent/internal/generated"
	"go.opentelemetry.io/otel"

	"go.opentelemetry.io/otel/propagation"
)

const (
	OtelCtx = "_otelCtx"
)

func ExtractOtelCtxFromMessage(ctx context.Context, order *generated.Order) context.Context {
	if header, ok := order.Metadata[OtelCtx]; !ok || ok && header == "" {
		logging.FromContext(ctx).Errorf("otel context not found")
		return ctx
	}

	carrier := propagation.MapCarrier{}
	if err := json.Unmarshal([]byte(order.Metadata[OtelCtx]), &carrier); err == nil {
		ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
		logging.FromContext(ctx).Debug("otel context extracted")
		return ctx
	}

	logging.FromContext(ctx).Error("cannot extract otel context")
	return ctx
}

func NewMsg(ctx context.Context) *generated.Message {
	metadata := make(map[string]string)
	message := &generated.Message{
		Metadata: metadata,
	}
	InjectOtelCtxInMessage(ctx, message)
	return message
}

func InjectOtelCtxInMessage(ctx context.Context, order *generated.Message) {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	otelContext, _ := json.Marshal(carrier)

	if order.Metadata == nil {
		order.Metadata = make(map[string]string)
	}
	order.Metadata[OtelCtx] = string(otelContext)
}
