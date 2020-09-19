COMMANDS=regctl
BINARIES=$(addprefix bin/,$(COMMANDS))
IMAGE_TAGS=regctl
IMAGES=$(addprefix docker-,$(IMAGE_TAGS))
GO_BUILD_FLAGS=

.PHONY: all binaries docker test .FORCE

.FORCE:

all: test binaries

test:
	go test ./...

binaries: $(BINARIES)

bin/regctl: .FORCE
	go build ${GO_BUILD_FLAGS} -o bin/regctl ./cmd/regctl

docker: $(IMAGES)

docker-regctl:
	docker build -t regclient/regctl -f build/Dockerfile.regctl .
