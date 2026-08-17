.PHONY: run build test vet fmt tidy swag docker-up docker-down migrate-create ci

run: ## Run the API locally (expects .env / env vars)
	go run ./cmd/server

build: ## Build the binary
	go build -o bin/chaos-api ./cmd/server

test: ## Run all tests
	go test ./... -count=1

vet: ## Static checks
	go vet ./...

fmt: ## Format
	gofmt -w .

tidy:
	go mod tidy

swag: ## Regenerate Swagger docs (requires swag CLI)
	swag init -g cmd/server/main.go -o docs/swagger --parseDependency --parseInternal

docker-up: ## Start postgres + redis + api
	docker compose up -d --build

docker-down:
	docker compose down

migrate-create: ## make migrate-create name=add_orders
	@test -n "$(name)" || (echo "usage: make migrate-create name=<snake_case>" && exit 1)
	@n=$$(printf "%06d" $$(( $$(ls migrations/*.up.sql 2>/dev/null | wc -l | tr -d ' ') + 1 ))); \
	touch migrations/$${n}_$(name).up.sql migrations/$${n}_$(name).down.sql; \
	echo "created migrations/$${n}_$(name).{up,down}.sql"

ci: vet test ## What CI runs
