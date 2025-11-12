set dotenv-load

ENVTEST_VERSION:="1.28.0"

default:
  @just --list

pc: pre-commit
pre-commit: tidy generate lint
tests: tests-unit tests-integration

generate:
  @go generate ./...

tidy: 
  #!/bin/bash
  set -euo pipefail
  go mod tidy &
  cd ./tests && go mod tidy &
  wait

lint:
  @golangci-lint run --fix --build-tags it --timeout 5m 
  @cd ./tests && golangci-lint run --fix --build-tags it --timeout 5m 


# TODO(fix): test using `-race`
tests-unit: lint generate
  #!/bin/bash
  set -euo pipefail
  mkdir -p ./coverage
  export KUBEBUILDER_ASSETS=$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@v0.0.0-20240320141353-395cfc7486e6 use {{ ENVTEST_VERSION }} -p path)
  go test -coverprofile=coverage/unit.txt -covermode=atomic ./internal/...
  cat ./coverage/unit.txt | grep -Ev "generated|pkg|web|tests/unit|with_trace|noop" > ./coverage/unit_filtered.txt

# TODO(fix): test using `--race`
tests-integration: lint generate
  #!/bin/bash
  set -euo pipefail
  mkdir -p ./coverage
  export KUBEBUILDER_ASSETS=$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@v0.0.0-20240320141353-395cfc7486e6 use {{ ENVTEST_VERSION }} -p path)
  ginkgo -r -p --output-interceptor-mode=none --output-dir=coverage --covermode atomic --cover --coverprofile=integration.txt --timeout "10m" --coverpkg=./internal/... ./tests
  cat ./coverage/integration.txt | grep -Ev "generated|pkg|web|tests/integration|with_trace|noop" > ./coverage/integration_filtered.txt

generate-agent-proto:
  @rm -rf ./internal/generated
  @mkdir -p ./internal/generated
  @protoc --go_out=./internal/generated --go_opt=paths=source_relative --go-grpc_out=./internal/generated --go-grpc_opt=paths=source_relative ./agent.proto


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

