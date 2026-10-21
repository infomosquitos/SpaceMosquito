.PHONY: build dev dev:firefox lint clean

build:
	npm run build

dev:
	npm run dev

dev:firefox:
	npx web-ext run --source-dir ./dist --target firefox

lint:
	npm run typecheck

clean:
	rm -rf dist
