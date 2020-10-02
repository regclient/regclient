COMMANDS=regctl
BINARIES=$(addprefix bin/,$(COMMANDS))
IMAGE_TAGS=regctl
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

vendor:
	go mod vendor

docker: $(IMAGES)

docker-regctl:
	docker build -t regclient/regctl -f build/Dockerfile.regctl .

plugin-user:
	mkdir -p ${HOME}/.docker/cli-plugins/
	cp docker-plugin/docker-regclient ${HOME}/.docker/cli-plugins/docker-regctl

plugin-host:
	sudo cp docker-plugin/docker-regclient /usr/libexec/docker/cli-plugins/docker-regctl
