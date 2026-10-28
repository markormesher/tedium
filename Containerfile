FROM docker.io/golang:1.27.3@sha256:921f61120011c774a7b37fd80bd3c3d259636090d3cd002b6a344322a0328082 as builder
WORKDIR /app

RUN apt update && apt install -y --no-install-recommends git

COPY go.mod go.sum ./
RUN go mod download

COPY ./.git ./.git
COPY ./cmd ./cmd
COPY ./internal ./internal

RUN go build -tags remote -ldflags "-X 'main.version=$(git describe --tags)'" -o ./build/main ./cmd

# ---

FROM docker.io/debian:13.7@sha256:4c25275446ca371766b8b093c696782eabce06263fccacb8d5d813b3dc44b308
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
