FROM ghcr.io/formancehq/base:22.04
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/agent /usr/bin/agent
ENV OTEL_SERVICE_NAME agent
ENTRYPOINT ["/usr/bin/agent"]
