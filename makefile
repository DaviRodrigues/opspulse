api-docs:
	apispec -c apispec.yaml -o ./docs/openapi.yaml

go-migrate:
	go run cmd/database/main.go

go-api:
	go run cmd/api/main.go

go-app:
	go run cmd/opspulse/main.go

test:
	go test -v ./...

sync-branches:
	bash ./scripts/merge.sh

up-dev:
	docker compose -f deployments/compose.dev.yml --profile dev up -d --build

up-api:
	docker compose -f deployments/compose.dev.yml --profile api up  -d --build

up-app:
	docker compose -f deployments/compose.dev.yml --profile app up  -d --build