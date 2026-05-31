FROM docker.io/golang:1.26.3@sha256:2d6c80227255c3112a4d08e67ba98e58efd3846daf15d9d7d4c389565d881b1a as builder
WORKDIR /app

RUN apt update && apt install -y --no-install-recommends libbtrfs-dev libgpgme-dev git

COPY go.mod go.sum ./
RUN go mod download

COPY ./.git ./.git
COPY ./cmd ./cmd
COPY ./internal ./internal

RUN go build -tags remote -ldflags "-X 'cmd.version=$(git describe --tags)'" -o ./build/main ./cmd

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
