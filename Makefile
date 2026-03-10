SHELL := /bin/sh

.PHONY: up down test test-backend build build-backend build-frontend compose-validate

up:
	docker compose up --build

down:
	docker compose down

test: test-backend

test-backend:
	cd backend && go test ./...

build: build-backend build-frontend

build-backend:
	cd backend && go build ./...

build-frontend:
	cd frontend && npm install && npm run build

compose-validate:
	docker compose config
