.DEFAULT_GOAL: build

.PHONY: lint
lint:
	gofmt -l . | (grep -v ^vendor/ || :)

.PHONY: build
build:
	go build -tags remote -o ./build/tedium ./cmd/tedium.go

.PHONY: run
run:
	go run -tags remote ./cmd/tedium.go --config ./config.yml

.PHONY: img
img:
	podman build -t tedium -f ./Containerfile
