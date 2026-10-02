api-docs:
	apispec -o docs/openapi.yaml

go-api:
	go run cmd/api/main.go

go-app:
	go run cmd/app/main.go

test:
	go test -v ./...

sync-branches:
	bash ./scripts/merge.sh

up-dev:
	docker compose -f deployments/compose.dev.yml --profile dev up

up-api:
	docker compose -f deployments/compose.dev.yml --profile api up

up-app:
	docker compose -f deployments/compose.dev.yml --profile app up