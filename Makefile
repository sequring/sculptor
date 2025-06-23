BINARY_NAME=sculptor
CMD_PATH=./cmd/sculptor-cli/main.go
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS = -ldflags "-X main.version=${VERSION} -X 'main.commit=$(shell git rev-parse HEAD)' -X 'main.date=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)'"

NS ?= prod
DEP ?= backend-fpm
CTX ?= om
RANGE ?= 7d
CONTAINER ?=

RUN_FLAGS = --namespace=${NS} --deployment=${DEP} --context=${CTX} --range=${RANGE}
ifeq ($(CONTAINER),)
else
RUN_FLAGS += --container=${CONTAINER}
endif

.PHONY: all
all: build

.PHONY: run
run:
	go run ${CMD_PATH} ${RUN_FLAGS}

.PHONY: build
build:
	go build -o ${BINARY_NAME} ${LDFLAGS} ${CMD_PATH}

.PHONY: test
test:
	go test -v ./...

.PHONY: coverage
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: clean
clean:
	rm -f ${BINARY_NAME}
	rm -f coverage.out

.PHONY: release
release:
	TAG_VERSION=$(shell git describe --tags `git rev-list --tags --max-count=1` | awk -F. '{print $$1"."$$2"."$$3+1}')
	git tag v${TAG_VERSION}
	git push origin v${TAG_VERSION}

.PHONY: help
help:
	@echo "Available commands:"
	@echo "  run        - Run the application with default/configurable parameters (e.g., make run NS=dev)"
	@echo "  build      - Build the application binary"
	@echo "  test       - Run all unit tests"
	@echo "  coverage   - Run tests and generate an HTML coverage report"
	@echo "  fmt        - Format all Go source files"
	@echo "  vet        - Run go vet to check for suspicious constructs"
	@echo "  clean      - Clean up build artifacts"
	@echo "  release    - Release a new version"
	@echo "  help       - Show this help message"