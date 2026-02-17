DATABASE_URL ?= postgres://kart:kart@localhost:5432/kart?sslmode=disable
TESTS_PATH   ?= ./...
COVERAGE_OUT ?= coverage.txt

setup: fmt-install lint-install dep ## Setup development tools and dependencies

## --- Build ---

generate: ## Generate code (OAS and SQL queries)
	go generate ./...
	sqlc generate
.PHONY: generate

build: ## Build all binaries
	go build ./...
.PHONY: build

## --- Quality ---

fmt-install: ## Install formatting tools
	go install github.com/daixiang0/gci@v0.13.7
	go install mvdan.cc/gofumpt@v0.9.2
.PHONY: fmt-install

fmt: ## Format the code (gci + gofumpt)
	gci write -s standard -s default -s "prefix(github.com/xenking/oolio-kart-challenge)" -s blank -s dot --skip-generated --custom-order cmd internal pkg
	gofumpt -l -w -extra cmd internal pkg
.PHONY: fmt

lint-install: ## Install golangci-lint
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.8.0
.PHONY: lint-install

lint: dep fmt ## Lint the code (runs dep + fmt first)
	golangci-lint run ./...
.PHONY: lint

test: ## Run tests with race detector and coverage
	go test -race $(TESTS_PATH) -coverprofile=$(COVERAGE_OUT) -covermode=atomic -coverpkg=$(TESTS_PATH)
.PHONY: test

test-cover: test ## Run tests and display coverage summary
	@go tool cover -func $(COVERAGE_OUT) | awk '/^total:/ {print "Total coverage: " $$3}'
.PHONY: test-cover

dep: ## Tidy and verify module dependencies
	go mod tidy && go mod verify
.PHONY: dep

## --- Database ---

seed-db: ## Seed database with products, coupons, and API key
	go run ./cmd/seed-db --database-url="$(DATABASE_URL)"
.PHONY: seed-db

download-coupons: ## Download coupon data files
	bash scripts/download-coupons.sh
.PHONY: download-coupons

ingest-coupons: download-coupons ## Download and ingest coupons into database
	go run ./cmd/coupon-ingest --database-url="$(DATABASE_URL)"
.PHONY: ingest-coupons

## --- Docker ---

up: ## Start docker compose (build + detach)
	docker compose up --build -d
.PHONY: up

down: ## Stop docker compose
	docker compose down
.PHONY: down

## --- Misc ---

cleanup-local-branches: ## Remove local branches whose remote is gone
	git remote prune origin
	git branch -vv | grep 'origin/.*: gone]' | awk '{print $$1}' | xargs git branch -D
.PHONY: cleanup-local-branches

# Absolutely awesome: http://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
