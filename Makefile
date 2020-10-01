COMMANDS=regctl
BINARIES=$(addprefix bin/,$(COMMANDS))
IMAGE_TAGS=regctl
IMAGES=$(addprefix docker-,$(IMAGE_TAGS))
GO_BUILD_FLAGS=

.PHONY: all binaries vendor docker test .FORCE

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
