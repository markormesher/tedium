FROM docker.io/golang:1.26.5@sha256:6cd10a6fcc5eadd62008fc2ad8056b38971cafd42f44d55297f18be8adc86410 as builder
WORKDIR /app

RUN apt update && apt install -y --no-install-recommends git

COPY go.mod go.sum ./
RUN go mod download

COPY ./.git ./.git
COPY ./cmd ./cmd
COPY ./internal ./internal

RUN go build -tags remote -ldflags "-X 'main.version=$(git describe --tags)'" -o ./build/main ./cmd

# ---

FROM docker.io/debian:13.6@sha256:fac46bff2e02f51425b6e33b0e1169f55dfb053d83511ca28aa50c09fd5ed7a4
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
