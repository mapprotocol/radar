# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.24.1-alpine AS build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath \
      -ldflags="-s -w -X github.com/mapprotocol/filter/internal/version.Version=$VERSION -X github.com/mapprotocol/filter/internal/version.Commit=$COMMIT -X github.com/mapprotocol/filter/internal/version.BuildDate=$BUILD_DATE" \
      -o /out/filter ./cmd

FROM gcr.io/distroless/static-debian12:nonroot

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.title="MAP Protocol Radar" \
      org.opencontainers.image.description="MAP Protocol cross-chain event filter" \
      org.opencontainers.image.source="https://github.com/mapprotocol/radar" \
      org.opencontainers.image.version=$VERSION \
      org.opencontainers.image.revision=$COMMIT \
      org.opencontainers.image.created=$BUILD_DATE

COPY --from=build /out/filter /usr/local/bin/filter

USER nonroot:nonroot
EXPOSE 9101

ENTRYPOINT ["/usr/local/bin/filter"]
CMD ["cli", "--config", "/etc/radar/config.json"]
