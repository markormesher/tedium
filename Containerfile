FROM docker.io/golang:1.23.3@sha256:73f06be4578c9987ce560087e2e2ea6485fb605e3910542cadd8fa09fc5f3e31 as builder
WORKDIR /app

RUN apt update && apt install -y --no-install-recommends libbtrfs-dev libgpgme-dev

# deps
COPY go.mod go.sum ./
RUN go mod download

# source code
COPY . .

RUN make build

# ---

FROM debian:bookworm@sha256:10901ccd8d249047f9761845b4594f121edef079cfd8224edebd9ea726f0a7f6
WORKDIR /app

RUN apt update \
  && apt install -y --no-install-recommends \
  ca-certificates \
  libbtrfs-dev \
  libgpgme-dev \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/build /app

CMD ["/app/tedium", "--config", "/tedium/config.yml"]
