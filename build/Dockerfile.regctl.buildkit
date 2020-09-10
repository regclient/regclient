# syntax=docker/dockerfile:experimental

FROM --platform=$BUILDPLATFORM golang:1.14-alpine as dev
RUN apk add --no-cache git ca-certificates
RUN adduser -D appuser
WORKDIR /src
COPY . /src/
CMD CGO_ENABLED=0 go build -ldflags '-s -w -extldflags -static' -o regctl ./cmd/regctl/ && ./regctl

FROM --platform=$BUILDPLATFORM dev as build
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,id=gomod,target=/go/pkg/mod/cache \
    --mount=type=cache,id=goroot,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags '-s -w -extldflags -static' -o regctl ./cmd/regctl/
USER appuser
CMD [ "./regctl" ]

FROM scratch as release
COPY --from=build /etc/passwd /etc/group /etc/
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /src/regctl /regctl
USER appuser
ENTRYPOINT [ "/regctl" ]

FROM --platform=$BUILDPLATFORM debian as debug
COPY --from=build /src/regctl /regctl
CMD [ "/regctl" ]

FROM scratch as artifact
COPY --from=build /src/regctl /regctl

FROM release
