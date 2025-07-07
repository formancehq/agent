VERSION 0.8

ARG core=github.com/formancehq/earthly:main
IMPORT $core AS core
IMPORT github.com/formancehq/operator:main AS operator

FROM core+base-image

sources:
    WORKDIR src
    COPY (operator+sources/*) /src
    WORKDIR /src
    COPY go.* .
    COPY --dir cmd internal pkg tests .
    COPY main.go .
    SAVE ARTIFACT /src

compile:
    FROM core+builder-image
    COPY (+sources/*) /src
    WORKDIR /src
    ARG VERSION=latest
    DO --pass-args core+GO_COMPILE --VERSION=$VERSION

build-image:
    FROM core+final-image
    ENTRYPOINT ["/bin/agent"]
    COPY (+compile/main) /bin/agent
    ARG REPOSITORY=ghcr.io
    ARG tag=latest
    DO core+SAVE_IMAGE --COMPONENT=agent --REPOSITORY=${REPOSITORY} --TAG=$tag

lint:
    FROM core+builder-image
    COPY (+sources/*) /src
    COPY --pass-args +tidy/go.* .
    WORKDIR /src
    DO --pass-args core+GO_LINT
    SAVE ARTIFACT cmd AS LOCAL cmd
    SAVE ARTIFACT internal AS LOCAL internal
    SAVE ARTIFACT main.go AS LOCAL main.go

openapi:
    RUN echo "not implemented"

tidy:
    FROM core+builder-image
    COPY --pass-args (+sources/src) /src
    WORKDIR /src
    DO --pass-args core+GO_TIDY

generate:
    FROM core+builder-image
    DO --pass-args core+GO_INSTALL --package=go.uber.org/mock/mockgen@latest
    COPY (+sources/*) /src
    WORKDIR /src    
    RUN go generate -run mockgen ./...
    SAVE ARTIFACT internal AS LOCAL internal


grpc-generate:
    FROM core+grpc-base
    LET protoName=agent.proto
    COPY $protoName .
    DO core+GRPC_GEN --protoName=$protoName
    SAVE ARTIFACT generated AS LOCAL internal/generated

tests:
    FROM core+builder-image
    RUN apk update && apk add bash
    DO --pass-args core+GO_INSTALL --package=sigs.k8s.io/controller-runtime/tools/setup-envtest@v0.0.0-20240320141353-395cfc7486e6
    ENV ENVTEST_VERSION 1.28.0
    RUN setup-envtest use $ENVTEST_VERSION -p path
    ENV KUBEBUILDER_ASSETS /root/.local/share/kubebuilder-envtest/k8s/$ENVTEST_VERSION-linux-$(go env GOHOSTARCH)
    DO --pass-args core+GO_INSTALL --package=github.com/onsi/ginkgo/v2/ginkgo@v2.22
    COPY --pass-args +sources/* /src
    COPY --pass-args operator+manifests/config /operator/config
    COPY (operator+sources/*) /src
    WORKDIR /src
    COPY tests tests
    COPY internal internal
    ARG GOPROXY
    ARG focus
    ENV DEBUG=true
    
    RUN --mount=type=cache,id=gomod,target=$GOPATH/pkg/mod \
        --mount=type=cache,id=gobuild,target=/root/.cache/go-build \
        go test ./internal/...

    RUN --mount=type=cache,id=gomod,target=$GOPATH/pkg/mod \
        --mount=type=cache,id=gobuild,target=/root/.cache/go-build \
        ginkgo --focus=$focus -p ./tests/...


deploy-staging:
    FROM --pass-args core+base-argocd 
    ARG --required TAG
    ARG APPLICATION=staging-eu-west-1-hosting-regions
    LET SERVER=argocd.internal.formance.cloud
    RUN --secret AUTH_TOKEN \
        argocd app set $APPLICATION \ 
        --parameter agent.image.tag=$TAG \
        --auth-token=$AUTH_TOKEN --server=$SERVER --grpc-web
    BUILD --pass-args core+deploy-staging

deploy:
    COPY (+sources/*) /src
    LET tag=$(tar cf - /src | sha1sum | awk '{print $1}')
    WAIT
        BUILD --pass-args +build-image --tag=$tag
    END
    FROM --pass-args core+vcluster-deployer-image

    ARG branch=main
    COPY --dir (github.com/formancehq/helm/charts/agent:$branch+validate/*) helm/
    COPY .earthly .earthly
    ARG --required user
    ARG --required REPOSITORY
    LET ADDITIONAL_ARGS=""
    ARG FORMANCE_DEV_CLUSTER_V2=no
    IF [ "$FORMANCE_DEV_CLUSTER_V2" == "yes" ]
        SET ADDITIONAL_ARGS="$ADDITIONAL_ARGS --set imagePullSecrets[0].name=zot"
        SET ADDITIONAL_ARGS="$ADDITIONAL_ARGS --set global.monitoring.traces.endpoint=otel-shared-admin.default.svc.cluster.local"
        SET ADDITIONAL_ARGS="$ADDITIONAL_ARGS --set global.monitoring.metrics.endpoint=otel-shared-admin.default.svc.cluster.local"
        SET ADDITIONAL_ARGS="$ADDITIONAL_ARGS --set image.repository=$REPOSITORY/formancehq/agent"
    END
    RUN --secret tld helm upgrade --namespace formance-system \
        --create-namespace \
        --install \
        --wait \
        -f .earthly/values.yaml \
        --set image.tag=$tag \
        --set server.tls.enabled=false \
        --set agent.baseUrl=https://$user.$tld \
        --set server.address=membership.formance.svc.cluster.local:8082 \
        formance-membership-agent ./helm $ADDITIONAL_ARGS

pre-commit:
    WAIT
        BUILD --pass-args +tidy
    END
    BUILD --pass-args +lint

release:
    FROM core+builder-image
    ARG mode=local
    COPY --dir . /src
    DO core+GORELEASER --mode=$mode