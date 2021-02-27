COMMANDS=regctl regsync regbot
BINARIES=$(addprefix bin/,$(COMMANDS))
IMAGE_TAGS=regctl regsync regbot
IMAGES=$(addprefix docker-,$(IMAGE_TAGS))
VCS_REF=$(shell git rev-list -1 HEAD)
VCS_TAG=$(shell git describe --tags --abbrev=0 2>/dev/null || echo "none")
LD_FLAGS=-X \"github.com/regclient/regclient/regclient.VCSRef=$(VCS_REF)\" \
         -X \"main.VCSRef=$(VCS_REF)\" -X \"main.VCSTag=$(VCS_TAG)\"
GO_BUILD_FLAGS=-ldflags "$(LD_FLAGS)"
DOCKER_ARGS=--build-arg "VCS_REF=$(VCS_REF)" --build-arg "LD_FLAGS=$(LD_FLAGS)"

.PHONY: all binaries vendor docker test plugin-user plugin-host .FORCE

.FORCE:

all: test binaries

test:
	go test ./...

binaries: vendor $(BINARIES)

bin/regctl: .FORCE
	go build ${GO_BUILD_FLAGS} -o bin/regctl ./cmd/regctl

bin/regsync: .FORCE
	go build ${GO_BUILD_FLAGS} -o bin/regsync ./cmd/regsync

bin/regbot: .FORCE
	go build ${GO_BUILD_FLAGS} -o bin/regbot ./cmd/regbot

vendor:
	go mod vendor

docker: $(IMAGES)

docker-regctl:
	docker build -t regclient/regctl -f build/Dockerfile.regctl $(DOCKER_ARGS) .
	docker build -t regclient/regctl:alpine -f build/Dockerfile.regctl --target release-alpine $(DOCKER_ARGS) .

docker-regsync:
	docker build -t regclient/regsync -f build/Dockerfile.regsync $(DOCKER_ARGS) .
	docker build -t regclient/regsync:alpine -f build/Dockerfile.regsync --target release-alpine $(DOCKER_ARGS) .

docker-regbot:
	docker build -t regclient/regbot -f build/Dockerfile.regbot $(DOCKER_ARGS) .
	docker build -t regclient/regbot:alpine -f build/Dockerfile.regbot --target release-alpine $(DOCKER_ARGS) .

plugin-user:
	mkdir -p ${HOME}/.docker/cli-plugins/
	cp docker-plugin/docker-regclient ${HOME}/.docker/cli-plugins/docker-regctl

plugin-host:
	sudo cp docker-plugin/docker-regclient /usr/libexec/docker/cli-plugins/docker-regctl
