FROM ghcr.io/formancehq/base:scratch
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/agent /usr/bin/agent
ENV OTEL_SERVICE_NAME agent
ENTRYPOINT ["/usr/bin/agent"]
