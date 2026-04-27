FROM docker.io/golang:1.26.2@sha256:b54cbf583d390341599d7bcbc062425c081105cc5ef6d170ced98ef9d047c716 as builder
WORKDIR /app

RUN apt update && apt install -y --no-install-recommends libbtrfs-dev libgpgme-dev

COPY go.mod go.sum ./
RUN go mod download

COPY ./cmd ./cmd
COPY ./internal ./internal

RUN go build -tags remote -o ./build/main ./cmd

# ---

FROM docker.io/debian:13.4@sha256:35b8ff74ead4880f22090b617372daff0ccae742eb5674455d542bef71ef1999
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
