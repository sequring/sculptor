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
	@echo "Checking for latest tag..."
	$(eval LATEST_TAG := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"))
	$(eval VERSION := $(subst v,,$(LATEST_TAG)))
	$(eval VERSION_PARTS := $(subst ., ,$(VERSION)))
	$(eval MAJOR := $(word 1,$(VERSION_PARTS)))
	$(eval MINOR := $(word 2,$(VERSION_PARTS)))
	$(eval PATCH := $(word 3,$(VERSION_PARTS)))
	$(eval NEXT_PATCH := $(shell echo $$(( $(PATCH) + 1 ))))
	$(eval NEXT_TAG := v$(MAJOR).$(MINOR).$(NEXT_PATCH))
	@echo "Current version: $(LATEST_TAG)"
	@echo "Next version:    $(NEXT_TAG)"
	@read -p "Is this correct? [y/N] " -n 1 -r; \
	echo; \
	if [ "$$REPLY" = "y" ] || [ "$$REPLY" = "Y" ]; then \
		TAG=$(NEXT_TAG); \
	else \
		read -p "Enter version (e.g., v1.2.3): " TAG; \
		if [ -z "$$TAG" ]; then \
			echo "No version specified. Release cancelled."; \
			exit 1; \
		fi; \
	fi; \
	if [ -n "$$TAG" ]; then \
		echo "Creating tag: $$TAG"; \
		git tag -a $$TAG -m "Release $$TAG" && \
		git push origin $$TAG; \
	else \
		echo "Release cancelled"; \
		exit 1; \
	fi

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