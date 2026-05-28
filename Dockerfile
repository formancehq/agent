FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG VERSION=latest
RUN CGO_ENABLED=0 go build -ldflags "-X github.com/formancehq/stack/components/agent/cmd.Version=${VERSION}" -o /bin/agent .

FROM ghcr.io/formancehq/base:22.04

COPY --from=builder /bin/agent /usr/bin/agent
ENV OTEL_SERVICE_NAME agent
ENTRYPOINT ["/usr/bin/agent"]
