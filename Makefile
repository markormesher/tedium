.DEFAULT_GOAL: build

.PHONY: lint
lint:
	gofmt -l . | (grep -v ^vendor/ || :)

.PHONY: build
build:
	go build -tags remote -o ./build/tedium ./cmd/tedium.go

.PHONY: run-github-app
run-github-app:
	go run -tags remote ./cmd/tedium.go --config ./config-github-app.json

.PHONY: run-github-user
run-github-user:
	go run -tags remote ./cmd/tedium.go --config ./config-github-user.json

.PHONY: run-gitea-user
run-gitea-user:
	go run -tags remote ./cmd/tedium.go --config ./config-gitea-user.yml

.PHONY: img
img:
	podman build -t tedium -f ./Containerfile
