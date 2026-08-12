.PHONY: up down migrate seed demo test load lint

up:
	docker compose up --build

down:
	docker compose down

migrate:
	docker compose run --rm migrate

seed:
	bash scripts/seed.sh

demo:
	bash scripts/demo.sh

test:
	go test ./...

load:
	bash scripts/load_test.sh

lint:
	golangci-lint run
