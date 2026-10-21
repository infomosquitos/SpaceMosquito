.PHONY: build run test migrate-up migrate-down lint dev-extension build-extensions clean

build:
	cd spacemosquito && go build -o spacemosquito ./cmd/spacemosquito

serve: build
	cd spacemosquito && ./spacemosquito serve

test:
	cd spacemosquito && go test -race ./...

migrate-up:
	cd spacemosquito && go run ./cmd/spacemosquito init

migrate-down:
	cd spacemosquito && go run ./cmd/spacemosquito migrate-down

lint:
	cd spacemosquito && go vet ./...
	cd firefox-extension && npm run typecheck
	cd chrome-extension && npm run typecheck

dev-extension:
	cd firefox-extension && npx web-ext run --source-dir ./dist --target firefox

build-extensions:
	./scripts/render-extension-icons.sh
	cd firefox-extension && npm ci && npm run build
	cd chrome-extension && npm ci && npm run build

clean:
	rm -f spacemosquito/spacemosquito
	cd spacemosquito && go clean
