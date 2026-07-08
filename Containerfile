FROM docker.io/golang:1.26.5@sha256:63f132d58c1f589f0dcda584933a9bb44bfda1150f1506377f5a902f34d86033 as builder
WORKDIR /app

RUN apt update && apt install -y --no-install-recommends git

COPY go.mod go.sum ./
RUN go mod download

COPY ./.git ./.git
COPY ./cmd ./cmd
COPY ./internal ./internal

RUN go build -tags remote -ldflags "-X 'main.version=$(git describe --tags)'" -o ./build/main ./cmd

# ---

FROM docker.io/debian:13.5@sha256:d07d1b51c39f51188e60be9b64e6bf769fa94e187f092bc32b91305cfa34ba5a
WORKDIR /app

RUN apt update \
  && apt install -y --no-install-recommends \
  ca-certificates \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/build/main /usr/local/bin/tedium

CMD ["/usr/local/bin/tedium", "--config", "/tedium/config.yml"]

LABEL image.name=markormesher/tedium
LABEL image.registry=ghcr.io
LABEL org.opencontainers.image.description=""
LABEL org.opencontainers.image.documentation=""
LABEL org.opencontainers.image.title="tedium"
LABEL org.opencontainers.image.url=""
LABEL org.opencontainers.image.vendor=""
LABEL org.opencontainers.image.version=""
