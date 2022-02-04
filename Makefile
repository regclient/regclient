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
GO_BUILD_FLAGS=-ldflags "$(LD_FLAGS)" -tags nolegacy
DOCKERFILE_EXT:=$(shell if docker build --help 2>/dev/null | grep -q -- '--progress'; then echo ".buildkit"; fi)
DOCKER_ARGS=--build-arg "VCS_REF=$(VCS_REF)"
GOPATH:=$(shell go env GOPATH)

.PHONY: all fmt vet test vendor binaries docker artifacts artifact-pre plugin-user plugin-host .FORCE

.FORCE:

all: fmt vet test lint binaries

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./...

lint: $(GOPATH)/bin/staticcheck
	$(GOPATH)/bin/staticcheck -checks all ./...

vendor:
	go mod vendor

embed/version.json: .FORCE
	# docker builds will not have the .dockerignore inside the container
	if [ -f ".dockerignore" -o ! -f "embed/version.json" ]; then \
		echo "{\"VCSRef\": \"$(VCS_REF)\", \"VCSTag\": \"$(VCS_TAG)\"}" >embed/version.json; \
	fi

binaries: vendor $(BINARIES)

bin/%: embed/version.json .FORCE
	if [ -f embed/version.json -a -d "cmd/$*/embed" ]; then cp embed/version.json "cmd/$*/embed/"; fi
	CGO_ENABLED=0 go build ${GO_BUILD_FLAGS} -o bin/$* ./cmd/$*

docker: $(IMAGES)

docker-%: embed/version.json .FORCE
	docker build -t regclient/$* -f build/Dockerfile.$*$(DOCKERFILE_EXT) $(DOCKER_ARGS) .
	docker build -t regclient/$*:alpine -f build/Dockerfile.$*$(DOCKERFILE_EXT) --target release-alpine $(DOCKER_ARGS) .

test-docker: $(addprefix test-docker-,$(COMMANDS))

test-docker-%:
	docker buildx build --platform="$(TEST_PLATFORMS)" -f build/Dockerfile.$*.buildkit .
	docker buildx build --platform="$(TEST_PLATFORMS)" -f build/Dockerfile.$*.buildkit --target release-alpine .

artifacts: $(ARTIFACTS)

artifact-pre:
	mkdir -p artifacts

artifacts/%: artifact-pre embed/version.json .FORCE
	@target="$*"; \
	command="$${target%%-*}"; \
	platform_ext="$${target#*-}"; \
	platform="$${platform_ext%.*}"; \
	export GOOS="$${platform%%-*}"; \
	export GOARCH="$${platform#*-}"; \
	echo export GOOS=$${GOOS}; \
	echo export GOARCH=$${GOARCH}; \
	echo go build ${GO_BUILD_FLAGS} -o "$@" ./cmd/$${command}/; \
	if [ -f embed/version.json -a -d "cmd/$${command}/embed" ]; then cp embed/version.json "cmd/$${command}/embed/"; fi; \
	CGO_ENABLED=0 go build ${GO_BUILD_FLAGS} -o "$@" ./cmd/$${command}/

plugin-user:
	mkdir -p ${HOME}/.docker/cli-plugins/
	cp docker-plugin/docker-regclient ${HOME}/.docker/cli-plugins/docker-regctl

plugin-host:
	sudo cp docker-plugin/docker-regclient /usr/libexec/docker/cli-plugins/docker-regctl

$(GOPATH)/bin/staticcheck: 
	go install "honnef.co/go/tools/cmd/staticcheck@latest"
