.PHONY: up down run test test-integration lint

up:
	docker compose up -d

down:
	docker compose down

run:
	go run .

test:
	go test ./... -short

test-integration:
	go test ./... -count=1

lint:
	golangci-lint run ./...
