FROM docker.io/golang:1.25.1@sha256:ab1f5c47de0f2693ed97c46a646bde2e4f380e40c173454d00352940a379af60 as builder
WORKDIR /app

RUN apt update && apt install -y --no-install-recommends libbtrfs-dev libgpgme-dev

COPY go.mod go.sum ./
RUN go mod download

COPY ./cmd ./cmd
COPY ./internal ./internal

RUN go build -tags remote -o ./build/main ./cmd

# ---

FROM debian:13.1@sha256:fd8f5a1df07b5195613e4b9a0b6a947d3772a151b81975db27d47f093f60c6e6
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
