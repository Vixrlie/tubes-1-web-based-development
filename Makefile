-include .env
export

.PHONY: up down run test lint

up:
	docker compose up -d

down:
	docker compose down

run:
	go run .

test:
	go test ./... -short

lint:
	golangci-lint run ./...
