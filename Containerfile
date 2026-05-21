FROM docker.io/golang:1.26.3@sha256:6df14f4a4bc9d979a3721f488981e0d1b318006377e473ed23d026796f5f4c0a as builder
WORKDIR /app

RUN apt update && apt install -y --no-install-recommends libbtrfs-dev libgpgme-dev

COPY go.mod go.sum ./
RUN go mod download

COPY ./cmd ./cmd
COPY ./internal ./internal

RUN go build -tags remote -o ./build/main ./cmd

# ---

FROM docker.io/debian:13.5@sha256:4ae67669760b807c19f23902a3fd7c121a6a70cf2ae709035674b23e712e4d62
WORKDIR /app

RUN apt update \
  && apt install -y --no-install-recommends \
  ca-certificates \
  libbtrfs-dev \
  libgpgme-dev \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/build/main /usr/local/bin/tedium

CMD ["/usr/local/bin/tedium", "--config", "/tedium/config.yml"]

LABEL image.name=markormesher/tedium
LABEL image.registry=ghcr.io
LABEL org.opencontainers.image.description=""
LABEL org.opencontainers.image.documentation=""
LABEL org.opencontainers.image.title="tedium"
LABEL org.opencontainers.image.url="https://github.com/markormesher/tedium"
LABEL org.opencontainers.image.vendor=""
LABEL org.opencontainers.image.version=""
