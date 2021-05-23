COMMANDS=regctl regsync regbot
BINARIES=$(addprefix bin/,$(COMMANDS))
IMAGES=$(addprefix docker-,$(COMMANDS))
ARTIFACT_PLATFORMS=linux-amd64 linux-arm64 darwin-amd64 windows-amd64.exe
ARTIFACTS=$(foreach cmd,$(addprefix artifacts/,$(COMMANDS)),$(addprefix $(cmd)-,$(ARTIFACT_PLATFORMS)))
VCS_REF=$(shell git rev-list -1 HEAD)
VCS_TAG=$(shell git describe --tags --abbrev=0 2>/dev/null || echo "none")
LD_FLAGS=-X \"github.com/regclient/regclient/regclient.VCSRef=$(VCS_REF)\" \
         -X \"main.VCSRef=$(VCS_REF)\" -X \"main.VCSTag=$(VCS_TAG)\" \
				 -s -w -extldflags -static
GO_BUILD_FLAGS=-ldflags "$(LD_FLAGS)"
DOCKERFILE_EXT=$(shell if docker build --help | grep -q -- '--progress'; then echo ".buildkit"; fi)
DOCKER_ARGS=--build-arg "VCS_REF=$(VCS_REF)"

.PHONY: all fmt vet test vendor binaries docker artifacts artifact-pre plugin-user plugin-host .FORCE

.FORCE:

all: fmt vet test binaries

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./...

vendor:
	go mod vendor

binaries: vendor $(BINARIES)

bin/%: .FORCE
	CGO_ENABLED=0 go build ${GO_BUILD_FLAGS} -o bin/$* ./cmd/$*

docker: $(IMAGES)

docker-%: .FORCE
	docker build -t regclient/$* -f build/Dockerfile.$*$(DOCKERFILE_EXT) $(DOCKER_ARGS) .
	docker build -t regclient/$*:alpine -f build/Dockerfile.$*$(DOCKERFILE_EXT) --target release-alpine $(DOCKER_ARGS) .

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
