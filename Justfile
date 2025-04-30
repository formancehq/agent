set dotenv-load

default:
  @just --list

pc: pre-commit
pre-commit: generate tidy lint
tests-all: tests-unit tests-integration

generate:
  @go generate ./...

tidy: 
  #!/bin/bash
  set -euo pipefail
  go mod tidy &
  cd ./tests && go mod tidy &
  wait

lint: tidy generate
  #!/bin/bash
  set -euo pipefail
  golangci-lint run --fix --build-tags it --timeout 5m &
  cd ./tests && golangci-lint run --fix --build-tags it --timeout 5m &
  wait

tests-unit: lint
  go test ./internal/...

tests-integration: lint
  #!/bin/bash
  set -euo pipefail
  ENVTEST_VERSION=1.28.0
  export KUBEBUILDER_ASSETS=$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@v0.0.0-20240320141353-395cfc7486e6 use $ENVTEST_VERSION -p path)
  ginkgo -p ./tests/... 

release-local:
  @goreleaser release --nightly --skip=publish --clean

release-ci:
  @goreleaser release --nightly --clean

release:
  @goreleaser release --clean

connect-dev:
  vcluster connect $USER --server=https://kube.$USER.formance.dev

uninstall: connect-dev
  helm uninstall agent -n formance-system || true

