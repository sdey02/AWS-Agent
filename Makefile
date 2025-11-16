.PHONY: help up down logs build clean test backend-test frontend-dev

help:
	@echo "AWS RAG Agent - Available Commands:"
	@echo ""
	@echo "  make up              - Start all services"
	@echo "  make down            - Stop all services"
	@echo "  make logs            - View logs from all services"
	@echo "  make build           - Build all Docker images"
	@echo "  make clean           - Remove all containers and volumes"
	@echo "  make test            - Run backend tests"
	@echo "  make backend-test    - Run backend tests locally"
	@echo "  make frontend-dev    - Start frontend in dev mode"
	@echo ""

up:
	@echo "Starting all services..."
	docker-compose up -d
	@echo "Services started! Frontend: http://localhost:3000, API: http://localhost:8080"

down:
	@echo "Stopping all services..."
	docker-compose down

logs:
	docker-compose logs -f

build:
	@echo "Building Docker images..."
	docker-compose build

clean:
	@echo "Cleaning up containers and volumes..."
	docker-compose down -v
	@echo "Cleanup complete"

test:
	@echo "Running tests..."
	docker-compose exec api go test ./... -v

backend-test:
	@echo "Running backend tests locally..."
	cd backend && go test ./... -v

frontend-dev:
	@echo "Starting frontend in dev mode..."
	cd frontend && npm install && npm run dev

init:
	@echo "Initializing project..."
	cp .env.example .env
	@echo "Please edit .env and add your OPENAI_API_KEY"
	@echo "Then run: make up"
