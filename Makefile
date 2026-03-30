.PHONY: up down build test logs clean

# Start the full stack
up:
	docker compose up -d
	@echo ""
	@echo "Stack is running!"
	@echo "  API:     http://localhost:8080"
	@echo "  Health:  http://localhost:8080/health"
	@echo ""
	@echo "Try it:"
	@echo '  curl -X POST http://localhost:8080/v1/scrape -H "Content-Type: application/json" -d '"'"'{"url":"https://example.com"}'"'"''

# Stop all services
down:
	docker compose down

# Build all Docker images
build:
	docker compose build

# Run tests
test:
	cd server && go test ./...

# View logs
logs:
	docker compose logs -f

# Clean everything including volumes
clean:
	docker compose down -v
