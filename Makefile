.PHONY: up down build test logs clean

# Start the full stack
up:
	docker compose up -d
	@echo ""
	@echo "Stack is running!"
	@echo "  API:     http://localhost:8080"
	@echo "  Browser: ws://localhost:9222/playwright"
	@echo "  API Key: sk_test_local_development_key_12345"

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
