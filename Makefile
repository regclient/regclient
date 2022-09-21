COMMANDS=regctl regsync regbot
BINARIES=$(addprefix bin/,$(COMMANDS))
IMAGES=$(addprefix docker-,$(COMMANDS))
ARTIFACT_PLATFORMS=linux-amd64 linux-arm64 linux-ppc64le linux-s390x darwin-amd64 darwin-arm64 windows-amd64.exe
ARTIFACTS=$(foreach cmd,$(addprefix artifacts/,$(COMMANDS)),$(addprefix $(cmd)-,$(ARTIFACT_PLATFORMS)))
TEST_PLATFORMS=linux/386,linux/amd64,linux/arm/v6,linux/arm/v7,linux/arm64,linux/ppc64le,linux/s390x
VCS_REF:=$(shell git rev-list -1 HEAD)
ifneq ($(shell git status --porcelain 2>/dev/null),)
  VCS_REF := $(VCS_REF)-dirty
endif
VCS_TAG:=$(shell git describe --tags --abbrev=0 2>/dev/null || true)
LD_FLAGS=-s -w -extldflags -static
GO_BUILD_FLAGS=-trimpath -ldflags "$(LD_FLAGS)" -tags nolegacy
DOCKERFILE_EXT:=$(shell if docker build --help 2>/dev/null | grep -q -- '--progress'; then echo ".buildkit"; fi)
DOCKER_ARGS=--build-arg "VCS_REF=$(VCS_REF)"
GOPATH:=$(shell go env GOPATH)
PWD:=$(shell pwd)

.PHONY: all fmt vet test lint lint-go lint-md vendor binaries docker artifacts artifact-pre plugin-user plugin-host .FORCE

.FORCE:

all: fmt vet test lint binaries

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test -cover ./...

lint: lint-go lint-md

lint-go: $(GOPATH)/bin/staticcheck .FORCE
	$(GOPATH)/bin/staticcheck -checks all ./...

lint-md: .FORCE
	docker run --rm -v "$(PWD):/workdir:ro" ghcr.io/igorshubovych/markdownlint-cli:latest \
	  --ignore vendor .

vendor:
	go mod vendor

binaries: vendor $(BINARIES)

bin/%: .FORCE
	CGO_ENABLED=0 go build ${GO_BUILD_FLAGS} -o bin/$* ./cmd/$*

docker: $(IMAGES)

docker-%: .FORCE
	docker build -t regclient/$* -f build/Dockerfile.$*$(DOCKERFILE_EXT) $(DOCKER_ARGS) .
	docker build -t regclient/$*:alpine -f build/Dockerfile.$*$(DOCKERFILE_EXT) --target release-alpine $(DOCKER_ARGS) .

test-docker: $(addprefix test-docker-,$(COMMANDS))

test-docker-%:
	docker buildx build --platform="$(TEST_PLATFORMS)" -f build/Dockerfile.$*.buildkit .
	docker buildx build --platform="$(TEST_PLATFORMS)" -f build/Dockerfile.$*.buildkit --target release-alpine .

artifacts: $(ARTIFACTS)

artifact-pre:
	mkdir -p artifacts

artifacts/%: artifact-pre .FORCE
	@target="$*"; \
	command="$${target%%-*}"; \
	platform_ext="$${target#*-}"; \
	platform="$${platform_ext%.*}"; \
	export GOOS="$${platform%%-*}"; \
	export GOARCH="$${platform#*-}"; \
	echo export GOOS=$${GOOS}; \
	echo export GOARCH=$${GOARCH}; \
	echo go build ${GO_BUILD_FLAGS} -o "$@" ./cmd/$${command}/; \
	CGO_ENABLED=0 go build ${GO_BUILD_FLAGS} -o "$@" ./cmd/$${command}/

plugin-user:
	mkdir -p ${HOME}/.docker/cli-plugins/
	cp docker-plugin/docker-regclient ${HOME}/.docker/cli-plugins/docker-regctl

plugin-host:
	sudo cp docker-plugin/docker-regclient /usr/libexec/docker/cli-plugins/docker-regctl

$(GOPATH)/bin/staticcheck: 
	go install "honnef.co/go/tools/cmd/staticcheck@latest"
