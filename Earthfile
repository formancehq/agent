VERSION 0.8

ARG core=github.com/formancehq/earthly:main
IMPORT $core AS core

FROM core+base-image

sources:
    WORKDIR /src
    COPY go.* .
    COPY --dir cmd internal pkg .
    COPY main.go .
    SAVE ARTIFACT /src

compile:
    FROM core+builder-image
    COPY (+sources/*) /src
    WORKDIR /src
    ARG VERSION=latest
    DO --pass-args core+GO_COMPILE --VERSION=$VERSION

build:
    FROM core+final-image
    ENTRYPOINT ["/bin/agent"]
    COPY (+compile/main) /bin/agent
    ARG REPOSITORY=ghcr.io
    ARG tag=latest
    DO core+SAVE_IMAGE --COMPONENT=agent --REPOSITORY=${REPOSITORY} --TAG=$tag

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
        BUILD --pass-args +build --tag=$tag
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
