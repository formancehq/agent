package internal

import "go.opentelemetry.io/otel"

var (
	tracer = otel.Tracer("com.formance.agent")
)
