VERSION 0.8

IMPORT github.com/formancehq/earthly:tags/v0.19.1 AS core

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

    ARG branch=agent-2.4.1
    COPY --dir (github.com/formancehq/helm/charts/agent:$branch+validate/*) helm/
    COPY .earthly .earthly
    ARG --required user
    RUN --secret tld helm upgrade --namespace formance-system \
        --create-namespace \
        --install \
        --wait \
        -f .earthly/values.yaml \
        --set image.tag=$tag \
        --set agent.baseUrl=https://$user.$tld \
        --set server.address=$user.$tld:443 \
        formance-membership-agent ./helm
