FROM docker.io/golang:1.23.3@sha256:d56c3e08fe5b27729ee3834854ae8f7015af48fd651cd25d1e3bcf3c19830174 as builder
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
