APP_NAME ?= hermes
IMAGE ?= quay.io/egevorky/$(APP_NAME):latest
GO ?= go

.PHONY: build run clean deps provision deprovision image-build image-push full-clean

build:
	$(GO) mod tidy
	$(GO) build -o bin/$(APP_NAME) ./cmd/hermes

run: build
	DB_HOST=localhost DB_PORT=5432 DB_USER=postgres DB_PASSWORD=postgres DB_NAME=hermes \
		./bin/$(APP_NAME)

clean:
	rm -rf bin

provision:
	podman-compose up -d

deprovision:
	podman-compose down -v --remove-orphans

image-build:
	podman build -f Containerfile -t $(IMAGE) .

image-push: image-build
	podman push $(IMAGE)
	$(MAKE) full-clean

full-clean: deprovision clean
	podman image prune -f
