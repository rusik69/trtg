.PHONY: build run test clean deps lint docker-build docker-build-web docker-run docker-stop deploy deploy-prune

# Variables
REMOTE_HOST := hetzner.govno2.cloud
REMOTE_USER := root
REMOTE_DIR := /opt/yttg
# Note: Server is IPv6-only. Use -6 flag to force IPv6 when connecting via hostname
SSH := ssh -6 $(REMOTE_USER)@$(REMOTE_HOST)
SCP := scp -6

# Build the application
build:
	go build -o bin/yttg .

# Run the application
run: build
	./bin/yttg

# Run with dry-run mode
dry-run: build
	./bin/yttg --dry-run

# Download dependencies
deps:
	go mod download
	go mod tidy

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf bin/
	rm -rf downloads/

# Lint the code
lint:
	golangci-lint run


# Docker commands
docker-build:
	docker build -t yttg .

docker-build-web:
	docker build -f Dockerfile.web -t yttg-web .

docker-run:
	docker-compose up -d telegram-bot-api
	@echo "Waiting for Telegram Bot API to start..."
	@sleep 5
	docker-compose run --rm yttg

docker-up:
	docker-compose up -d

docker-stop:
	docker-compose down

docker-logs:
	docker-compose logs -f

# Run Local Bot API Server standalone
run-local-api:
	docker run -d -p 8081:8081 --name=telegram-bot-api \
		-e TELEGRAM_API_ID=$${TELEGRAM_API_ID} \
		-e TELEGRAM_API_HASH=$${TELEGRAM_API_HASH} \
		aiogram/telegram-bot-api:latest

stop-local-api:
	docker stop telegram-bot-api
	docker rm telegram-bot-api

# Deployment commands
deploy:
	@echo "Deploying to $(REMOTE_HOST)..."
	@test -f prod.env || (echo "Error: prod.env file not found!" && exit 1)
	@echo "Copying files..."
	$(SSH) "mkdir -p $(REMOTE_DIR)"
	$(SCP) Dockerfile $(REMOTE_USER)@$(REMOTE_HOST):$(REMOTE_DIR)/
	$(SCP) Dockerfile.web $(REMOTE_USER)@$(REMOTE_HOST):$(REMOTE_DIR)/
	$(SCP) docker-compose.yml $(REMOTE_USER)@$(REMOTE_HOST):$(REMOTE_DIR)/
	$(SCP) torrents.txt $(REMOTE_USER)@$(REMOTE_HOST):$(REMOTE_DIR)/
	$(SCP) go.mod go.sum $(REMOTE_USER)@$(REMOTE_HOST):$(REMOTE_DIR)/
	$(SCP) main.go $(REMOTE_USER)@$(REMOTE_HOST):$(REMOTE_DIR)/
	$(SCP) -r pkg $(REMOTE_USER)@$(REMOTE_HOST):$(REMOTE_DIR)/
	$(SCP) -r cmd $(REMOTE_USER)@$(REMOTE_HOST):$(REMOTE_DIR)/
	$(SCP) prod.env $(REMOTE_USER)@$(REMOTE_HOST):$(REMOTE_DIR)/.env
	@echo "Building and starting services..."
	$(SSH) "cd $(REMOTE_DIR) && docker-compose down || true"
	$(SSH) "cd $(REMOTE_DIR) && docker-compose build --no-cache"
	$(SSH) "cd $(REMOTE_DIR) && docker-compose up -d"
	@echo "Deployment complete!"

deploy-setup:
	@echo "Setting up remote server..."
	$(SSH) "mkdir -p $(REMOTE_DIR)"
	$(SSH) "apt-get update && apt-get install -y docker.io docker-compose"
	@echo "Configuring Docker for IPv6-only..."
	$(SSH) "mkdir -p /etc/docker"
	@echo '{"dns":["2001:4860:4860::8888","2001:4860:4860::8844","2606:4700:4700::1111"],"ipv6":true,"fixed-cidr-v6":"2001:db8:1::/64","experimental":true,"ip6tables":true}' | $(SSH) "cat > /etc/docker/daemon.json"
	$(SSH) "systemctl daemon-reload"
	$(SSH) "systemctl restart docker"
	$(SSH) "systemctl enable docker"

deploy-configure-docker:
	@echo "Configuring Docker for IPv6-only on $(REMOTE_HOST)..."
	$(SSH) "mkdir -p /etc/docker"
	@echo '{"dns":["2001:4860:4860::8888","2001:4860:4860::8844","2606:4700:4700::1111"],"ipv6":true,"fixed-cidr-v6":"2001:db8:1::/64","experimental":true,"ip6tables":true}' | $(SSH) "cat > /etc/docker/daemon.json"
	$(SSH) "systemctl daemon-reload && systemctl restart docker"

deploy-stop:
	@echo "Stopping services on $(REMOTE_HOST)..."
	$(SSH) "cd $(REMOTE_DIR) && docker-compose down"

deploy-prune:
	@echo "Pruning Docker system and volumes on $(REMOTE_HOST)..."
	@echo "Cleaning up Docker system..."
	$(SSH) "docker system prune -a -f"
	@echo "Cleaning up Docker volumes..."
	$(SSH) "docker volume prune -a -f"
	@echo "Pruning complete!"

deploy-logs:
	$(SSH) "cd $(REMOTE_DIR) && docker-compose logs -f"

deploy-status:
	$(SSH) "cd $(REMOTE_DIR) && docker-compose ps"

deploy-run-once:
	@echo "Running yttg once on $(REMOTE_HOST)..."
	$(SSH) "cd $(REMOTE_DIR) && docker-compose run --rm yttg"

deploy-clean:
	@echo "Cleaning up remote server..."
	$(SSH) "cd $(REMOTE_DIR) && docker-compose down -v"
	$(SSH) "rm -rf $(REMOTE_DIR)"

# Show help
help:
	@echo "Available targets:"
	@echo ""
	@echo "Local development:"
	@echo "  build              - Build the application"
	@echo "  run                - Build and run the application"
	@echo "  dry-run            - Run in dry-run mode (no downloads)"
	@echo "  deps               - Download dependencies"
	@echo "  test               - Run tests"
	@echo "  clean              - Clean build artifacts"
	@echo "  lint               - Run linter"
	@echo ""
	@echo "Docker commands:"
	@echo "  docker-build       - Build Docker image for yttg"
	@echo "  docker-build-web   - Build Docker image for yttg-web"
	@echo "  docker-run         - Run with docker-compose"
	@echo "  docker-up          - Start all services in background"
	@echo "  docker-stop        - Stop all services"
	@echo "  docker-logs        - Show logs"
	@echo "  run-local-api      - Start Local Bot API Server standalone"
	@echo "  stop-local-api     - Stop Local Bot API Server"
	@echo ""
	@echo "Deployment (to $(REMOTE_HOST)):"
	@echo "  deploy             - Full deployment (copy + build + start)"
	@echo "  deploy-setup       - Initial server setup (install Docker)"
	@echo "  deploy-configure-docker - Configure Docker for IPv6-only"
	@echo "  deploy-stop        - Stop services on server"
	@echo "  deploy-prune       - Prune Docker system and volumes on server"
	@echo "  deploy-logs        - Show logs from server"
	@echo "  deploy-status      - Show service status"
	@echo "  deploy-run-once    - Run yttg once on server"
	@echo "  deploy-clean       - Remove everything from server"
	@echo ""
	@echo "Environment variables:"
	@echo "  TELEGRAM_BOT_TOKEN - Telegram bot token (required)"
	@echo "  TELEGRAM_CHAT_ID   - Chat ID to send videos to (required)"
	@echo "  TELEGRAM_API_URL   - Local Bot API Server URL (required)"
	@echo "  TELEGRAM_API_ID    - Telegram API ID (for Local Bot API Server)"
	@echo "  TELEGRAM_API_HASH  - Telegram API Hash (for Local Bot API Server)"
	@echo "  WEB_USERNAME        - Web interface username (default: admin)"
	@echo "  WEB_PASSWORD        - Web interface password (default: admin)"
	@echo ""
	@echo "Files:"
	@echo "  prod.env           - Production environment variables (not in git)"
	@echo "  torrents.txt       - Torrent URLs (magnet links or .torrent file paths)"
