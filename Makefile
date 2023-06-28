COMMANDS?=regctl regsync regbot
BINARIES?=$(addprefix bin/,$(COMMANDS))
IMAGES?=$(addprefix docker-,$(COMMANDS))
ARTIFACT_PLATFORMS?=linux-amd64 linux-arm64 linux-ppc64le linux-s390x darwin-amd64 darwin-arm64 windows-amd64.exe
ARTIFACTS?=$(foreach cmd,$(addprefix artifacts/,$(COMMANDS)),$(addprefix $(cmd)-,$(ARTIFACT_PLATFORMS)))
TEST_PLATFORMS?=linux/386,linux/amd64,linux/arm/v6,linux/arm/v7,linux/arm64,linux/ppc64le,linux/s390x
VCS_REPO?="https://github.com/regclient/regclient.git"
VCS_REF?=$(shell git rev-list -1 HEAD)
ifneq ($(shell git status --porcelain 2>/dev/null),)
  VCS_REF := $(VCS_REF)-dirty
endif
VCS_TAG?=$(shell git describe --tags --abbrev=0 2>/dev/null || true)
LD_FLAGS?=-s -w -extldflags -static -buildid= -X \"github.com/regclient/regclient/internal/version.vcsTag=$(VCS_TAG)\"
GO_BUILD_FLAGS?=-trimpath -ldflags "$(LD_FLAGS)" -tags nolegacy
DOCKERFILE_EXT?=$(shell if docker build --help 2>/dev/null | grep -q -- '--progress'; then echo ".buildkit"; fi)
DOCKER_ARGS?=--build-arg "VCS_REF=$(VCS_REF)"
GOPATH?=$(shell go env GOPATH)
PWD:=$(shell pwd)
VER_BUMP?=$(shell command -v version-bump 2>/dev/null)
VER_BUMP_CONTAINER?=sudobmitch/version-bump:edge
ifeq "$(strip $(VER_BUMP))" ''
	VER_BUMP=docker run --rm \
		-v "$(shell pwd)/:$(shell pwd)/" -w "$(shell pwd)" \
		-u "$(shell id -u):$(shell id -g)" \
		$(VER_BUMP_CONTAINER)
endif
MARKDOWN_LINT_VER?=v0.35.0
SYFT?=$(shell command -v syft 2>/dev/null)
SYFT_CMD_VER:=$(shell [ -x "$(SYFT)" ] && echo "v$$($(SYFT) version | awk '/^Version: / {print $$2}')" || echo "0")
SYFT_VERSION?=v0.84.0
SYFT_CONTAINER?=anchore/syft:v0.84.0@sha256:bd5ef3acf08b5be457905430f8fcda530d907bb173bf086e4749acb41077acb1
ifneq "$(SYFT_CMD_VER)" "$(SYFT_VERSION)"
	SYFT=docker run --rm \
		-v "$(shell pwd)/:$(shell pwd)/" -w "$(shell pwd)" \
		-u "$(shell id -u):$(shell id -g)" \
		$(SYFT_CONTAINER)
endif
STATICCHECK_VER?=v0.4.3

.PHONY: .FORCE
.FORCE:

.PHONY: all
all: fmt vet test lint binaries ## Full build of Go binaries (including fmt, vet, test, and lint)

.PHONY: fmt
fmt: ## go fmt
	go fmt ./...

.PHONY: vet
vet: ## go vet
	go vet ./...

.PHONY: test
test: ## go test
	go test -cover -race ./...

.PHONY: lint
lint: lint-go lint-md ## Run all linting

.PHONY: lint-go
lint-go: $(GOPATH)/bin/staticcheck .FORCE ## Run linting for Go
	$(GOPATH)/bin/staticcheck -checks all ./...

.PHONY: lint-md
lint-md: .FORCE ## Run linting for markdown
	docker run --rm -v "$(PWD):/workdir:ro" ghcr.io/igorshubovych/markdownlint-cli:$(MARKDOWN_LINT_VER) \
	  --ignore vendor .

.PHONY: vulncheck-go
vulncheck-go: $(GOPATH)/bin/govulncheck .FORCE ## Run vulnerability scan for Go
	$(GOPATH)/bin/govulncheck ./...

.PHONY: vendor
vendor: ## Vendor Go modules
	go mod vendor

.PHONY: binaries
binaries: vendor $(BINARIES) ## Build Go binaries

bin/%: .FORCE
	CGO_ENABLED=0 go build ${GO_BUILD_FLAGS} -o bin/$* ./cmd/$*

.PHONY: docker
docker: $(IMAGES) ## Build Docker images

docker-%: .FORCE
	docker build -t regclient/$* -f build/Dockerfile.$*$(DOCKERFILE_EXT) $(DOCKER_ARGS) .
	docker build -t regclient/$*:alpine -f build/Dockerfile.$*$(DOCKERFILE_EXT) --target release-alpine $(DOCKER_ARGS) .

.PHONY: oci-image
oci-image: $(addprefix oci-image-,$(COMMANDS)) ## Build reproducible images to an OCI Layout

oci-image-%: bin/regctl .FORCE
	build/oci-image.sh -r scratch -i "$*" -p "$(TEST_PLATFORMS)"
	build/oci-image.sh -r alpine  -i "$*" -p "$(TEST_PLATFORMS)" -b "alpine:3"

.PHONY: test-docker
test-docker: $(addprefix test-docker-,$(COMMANDS)) ## Build multi-platform docker images (but do not tag)

test-docker-%:
	docker buildx build --platform="$(TEST_PLATFORMS)" -f build/Dockerfile.$*.buildkit .
	docker buildx build --platform="$(TEST_PLATFORMS)" -f build/Dockerfile.$*.buildkit --target release-alpine .

.PHONY: ci-distribution
ci-distribution:
	docker run --rm -d -p 5000 \
		--label regclient-ci=true --name regclient-ci-distribution \
		-e "REGISTRY_STORAGE_DELETE_ENABLED=true" \
		docker.io/registry:2.8.2
	./build/ci-test.sh -t localhost:$$(docker port regclient-ci-distribution 5000 | head -1 | cut -f2 -d:)/test-ci
	docker stop regclient-ci-distribution

.PHONY: ci-zot
ci-zot:
	docker run --rm -d -p 5000 \
		--label regclient-ci=true --name regclient-ci-zot \
		-v "$$(pwd)/build/zot-config.json:/etc/zot/config.json:ro" \
		ghcr.io/project-zot/zot-linux-amd64:v2.0.0-rc5
	./build/ci-test.sh -t localhost:$$(docker port regclient-ci-zot 5000 | head -1 | cut -f2 -d:)/test-ci
	docker stop regclient-ci-zot

.PHONY: artifacts
artifacts: $(ARTIFACTS) ## Generate artifacts

.PHONY: artifact-pre
artifact-pre:
	mkdir -p artifacts

artifacts/%: artifact-pre .FORCE
	@set -e; \
	target="$*"; \
	command="$${target%%-*}"; \
	platform_ext="$${target#*-}"; \
	platform="$${platform_ext%.*}"; \
	export GOOS="$${platform%%-*}"; \
	export GOARCH="$${platform#*-}"; \
	echo export GOOS=$${GOOS}; \
	echo export GOARCH=$${GOARCH}; \
	echo go build ${GO_BUILD_FLAGS} -o "$@" ./cmd/$${command}/; \
	CGO_ENABLED=0 go build ${GO_BUILD_FLAGS} -o "$@" ./cmd/$${command}/; \
	$(SYFT) packages -q "file:$@" --source-name "$${command}" -o cyclonedx-json >"artifacts/$${command}-$${platform}.cyclonedx.json"; \
	$(SYFT) packages -q "file:$@" --source-name "$${command}" -o spdx-json >"artifacts/$${command}-$${platform}.spdx.json"

.PHONY: plugin-user
plugin-user:
	mkdir -p ${HOME}/.docker/cli-plugins/
	cp docker-plugin/docker-regclient ${HOME}/.docker/cli-plugins/docker-regctl

.PHONY: plugin-host
plugin-host:
	sudo cp docker-plugin/docker-regclient /usr/libexec/docker/cli-plugins/docker-regctl

.PHONY: util-golang-update
util-golang-update: ## update go module versions
	go get -u
	go mod tidy
	go mod vendor
	
.PHONY: util-version-check
util-version-check: ## check all dependencies for updates
	$(VER_BUMP) check

.PHONY: util-version-update
util-version-update: ## update versions on all dependencies
	$(VER_BUMP) update

$(GOPATH)/bin/staticcheck: .FORCE
	@[ -f $(GOPATH)/bin/staticcheck ] \
	&& [ "$$($(GOPATH)/bin/staticcheck -version | cut -f 3 -d ' ' | tr -d '()')" = "$(STATICCHECK_VER)" ] \
	|| go install "honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VER)"

$(GOPATH)/bin/govulncheck: .FORCE
	# for now, keep installing the latest until they start releasing versions
	go install "golang.org/x/vuln/cmd/govulncheck@latest"

.PHONY: help
help: # Display help
	@awk -F ':|##' '/^[^\t].+?:.*?##/ { printf "\033[36m%-30s\033[0m %s\n", $$1, $$NF }' $(MAKEFILE_LIST)
