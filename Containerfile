FROM docker.io/golang:1.24.4@sha256:20a022e5112a144aa7b7aeb3f22ebf2cdaefcc4aac0d64e8deeee8cdc18b9c0f as builder
WORKDIR /app

RUN apt update && apt install -y --no-install-recommends libbtrfs-dev libgpgme-dev

COPY go.mod go.sum ./
RUN go mod download

COPY ./cmd ./cmd
COPY ./internal ./internal

RUN go build -tags remote -o ./build/main ./cmd

# ---

FROM debian:bookworm@sha256:d42b86d7e24d78a33edcf1ef4f65a20e34acb1e1abd53cabc3f7cdf769fc4082
WORKDIR /app

LABEL image.registry=ghcr.io
LABEL image.name=markormesher/tedium

RUN apt update \
  && apt install -y --no-install-recommends \
  ca-certificates \
  libbtrfs-dev \
  libgpgme-dev \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/build/main /usr/local/bin/tedium

CMD ["/usr/local/bin/tedium", "--config", "/tedium/config.yml"]
