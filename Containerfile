FROM docker.io/golang:1.23.2@sha256:adee809c2d0009a4199a11a1b2618990b244c6515149fe609e2788ddf164bd10 as builder
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
