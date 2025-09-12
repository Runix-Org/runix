.PHONY: build run ui-lint ui-lint-fix ui-format ui-postclone ui-build-debug ui-build-release ui-dev help

build: ## Build
	@go build -ldflags "-s -w"

run: build
	@./runix

ui-lint: ## Lint frontend
	@(cd frontend && npm run lint)

ui-lint-fix: ## Fix frontend lint
	@(cd frontend && npm run lint:fix)

ui-format: ## Format frontend
	@(cd frontend && npm run format)

ui-postclone: ## Install frontend deps
	@rm -rf ./frontend/node_modules
	@(cd frontend && npm install)

ui-build-debug: ## Build frontend for debug
	@(cd frontend && npm run build:debug)

ui-build-release: ## Build frontend for release
	@(cd frontend && npm run build)

ui-dev: ## Run frontend in dev mode
	@(cd frontend && npm run dev)

help: ## Display this help screen
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
