VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS  = -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: all build run test lint docker clean

all: build

build:
	CGO_ENABLED=1 go build -ldflags="$(LDFLAGS)" -o silentmap ./cmd/silentmap

build-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=1 \
	CC=aarch64-linux-gnu-gcc \
	go build -ldflags="$(LDFLAGS)" -o silentmap-linux-arm64 ./cmd/silentmap

run:
	go run ./cmd/silentmap --debug

test:
	go test ./... -v -race

lint:
	golangci-lint run ./...

docker:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		-t silentmap/silentmap:$(VERSION) \
		-t silentmap/silentmap:latest \
		.

docker-up:
	docker compose up -d

docker-logs:
	docker compose logs -f silentmap

docker-down:
	docker compose down

# OUI-Datenbank aktualisieren (IEEE-Download)
update-oui:
	./scripts/update-oui.sh

clean:
	rm -f silentmap silentmap-linux-*
	go clean -cache
