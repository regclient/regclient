COMMANDS=regctl regsync
BINARIES=$(addprefix bin/,$(COMMANDS))
IMAGE_TAGS=regctl regsync
IMAGES=$(addprefix docker-,$(IMAGE_TAGS))
GO_BUILD_FLAGS=

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

vendor:
	go mod vendor

docker: $(IMAGES)

docker-regctl:
	docker build -t regclient/regctl -f build/Dockerfile.regctl .
	docker build -t regclient/regctl:alpine -f build/Dockerfile.regctl --target release-alpine .

docker-regsync:
	docker build -t regclient/regsync -f build/Dockerfile.regsync .
	docker build -t regclient/regsync:alpine -f build/Dockerfile.regsync --target release-alpine .

plugin-user:
	mkdir -p ${HOME}/.docker/cli-plugins/
	cp docker-plugin/docker-regclient ${HOME}/.docker/cli-plugins/docker-regctl

plugin-host:
	sudo cp docker-plugin/docker-regclient /usr/libexec/docker/cli-plugins/docker-regctl
