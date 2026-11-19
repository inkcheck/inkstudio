# InkStudio. Run `make` or `make help` for the list of targets.

.DEFAULT_GOAL := help
.PHONY: help frontend build run web snap screenshots site serve test app notarize dmg install release-check release icon clean

export MACOSX_DEPLOYMENT_TARGET := 13.0
export CGO_LDFLAGS := -mmacosx-version-min=13.0

help: ## Show this help
	@awk 'BEGIN { FS = ":.*## " } \
		/^[a-zA-Z0-9_-]+:.*## / { printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

frontend/node_modules: frontend/package.json
	cd frontend && pnpm install
	@touch $@

frontend: frontend/node_modules ## Build the frontend into frontend/dist
	cd frontend && pnpm run build

build: frontend ## Build bin/inkstudio
	go build -o bin/inkstudio .

run: build ## Build and run; PACK=path/to/pack opens a pack
	bin/inkstudio $(PACK)

web: frontend ## Serve the UI to a browser on :1416; PACK=path/to/pack, or the sample deck
	go run ./cmd/inkstudio-web $(or $(PACK),-sample)

bin/webkit-snap: tools/webkit-snap/snap.swift
	@mkdir -p bin
	swiftc -O $< -o $@

snap: bin/webkit-snap ## Snapshot `make web` in WebKit (macOS): OUT=file.png
	bin/webkit-snap http://127.0.0.1:1416/ $(or $(OUT),snap.png) 1440 900 3

screenshots: ## Take the site's screenshots of the UI, light and dark (macOS)
	./scripts/screenshots.sh

site: ## Build the site into site/public
	cd site && hugo --gc --minify

serve: ## Serve the site on http://localhost:1418/inkstudio/
	cd site && hugo server --port 1418

test: ## Run the Go tests and type-check the frontend
	go test ./...
	cd frontend && pnpm run check

app: ## Build build/InkStudio.app: universal, ad-hoc signed, for this Mac
	./scripts/package.sh app

notarize: app ## Sign the app with Developer ID, notarise and staple it
	./scripts/package.sh notarize

dmg: app ## Make build/InkStudio-<version>.dmg, signed, notarised and stapled
	./scripts/package.sh dmg

install: app ## Notarise the app and install it in /Applications
	./scripts/package.sh install

release-check: ## Check everything a release needs, without releasing
	./scripts/release.sh check

release: ## Release the tagged HEAD: a notarised DMG on GitHub, and the Homebrew cask
	./scripts/release.sh publish

icon: ## Redraw packaging/inkstudio.icns
	@mkdir -p bin
	swiftc -O tools/make-icon/icon.swift -o bin/make-icon
	rm -rf bin/inkstudio.iconset && bin/make-icon bin/inkstudio.iconset
	iconutil -c icns bin/inkstudio.iconset -o packaging/inkstudio.icns

clean: ## Remove build output
	rm -rf bin build frontend/dist/assets frontend/dist/index.html
