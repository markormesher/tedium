FROM docker.io/golang:1.22.6@sha256:2bd56f00ff47baf33e64eae7996b65846c7cb5e0a46e0a882ef179fd89654afa as builder
WORKDIR /app

# deps
COPY go.mod go.sum ./
RUN go mod download

# source code
COPY . .

RUN make build

# ---

FROM debian:bookworm@sha256:27586f4609433f2f49a9157405b473c62c3cb28a581c413393975b4e8496d0ab
WORKDIR /app

RUN apt update \
  && apt install -y --no-install-recommends \
  ca-certificates \
  && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/build /app

CMD ["/app/tedium", "--config", "/tedium/config.yml"]
