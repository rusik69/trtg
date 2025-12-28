.PHONY: build run test clean deps lint docker-build docker-build-web docker-run docker-stop deploy deploy-start deploy-stop deploy-restart deploy-prune help

REMOTE_HOST := hetzner.govno2.cloud
REMOTE_USER := ubuntu
REMOTE_DIR := /opt/trtg
REMOTE := $(REMOTE_USER)@$(REMOTE_HOST)
SSH := ssh -6 $(REMOTE)
SCP := scp -6

build:
	go build -o bin/trtg ./cmd/trtg

run: build
	./bin/trtg

dry-run: build
	./bin/trtg --dry-run

deps:
	go mod download && go mod tidy

test:
	go test -v ./...

clean:
	rm -rf bin/ downloads/

lint:
	golangci-lint run


docker-build:
	docker build -t trtg .

docker-build-web:
	docker build -f Dockerfile.web -t trtg-web .

docker-run:
	docker-compose up -d telegram-bot-api
	@sleep 5
	docker-compose run --rm trtg

docker-up:
	docker-compose up -d

docker-stop:
	docker-compose down

docker-logs:
	docker-compose logs -f

run-local-api:
	docker run -d -p 8081:8081 --name=telegram-bot-api \
		-e TELEGRAM_API_ID=$${TELEGRAM_API_ID} \
		-e TELEGRAM_API_HASH=$${TELEGRAM_API_HASH} \
		aiogram/telegram-bot-api:latest

stop-local-api:
	docker stop telegram-bot-api && docker rm telegram-bot-api

deploy:
	@test -f prod.env || (echo "Error: prod.env file not found!" && exit 1)
	$(SSH) "sudo mkdir -p $(REMOTE_DIR) && sudo chown -R $(REMOTE_USER):$(REMOTE_USER) $(REMOTE_DIR)"
	$(SCP) Dockerfile Dockerfile.web docker-compose.yml torrents.txt go.mod go.sum $(REMOTE):$(REMOTE_DIR)/
	$(SCP) -r pkg cmd nginx $(REMOTE):$(REMOTE_DIR)/
	$(SCP) prod.env $(REMOTE):$(REMOTE_DIR)/.env
	$(SSH) "cd $(REMOTE_DIR) && sudo docker-compose down || true && sudo docker-compose build --no-cache && sudo docker-compose up -d"

deploy-setup:
	$(SSH) "mkdir -p $(REMOTE_DIR) && apt-get update && apt-get install -y docker.io docker-compose"
	@echo '{"dns":["2001:4860:4860::8888","2001:4860:4860::8844","2606:4700:4700::1111"],"ipv6":true,"fixed-cidr-v6":"2001:db8:1::/64","experimental":true,"ip6tables":true}' | $(SSH) "mkdir -p /etc/docker && cat > /etc/docker/daemon.json"
	$(SSH) "systemctl daemon-reload && systemctl restart docker && systemctl enable docker"

deploy-configure-docker:
	@echo '{"dns":["2001:4860:4860::8888","2001:4860:4860::8844","2606:4700:4700::1111"],"ipv6":true,"fixed-cidr-v6":"2001:db8:1::/64","experimental":true,"ip6tables":true}' | $(SSH) "mkdir -p /etc/docker && cat > /etc/docker/daemon.json"
	$(SSH) "systemctl daemon-reload && systemctl restart docker"

deploy-stop:
	$(SSH) "cd $(REMOTE_DIR) && sudo docker-compose down"

deploy-start:
	$(SSH) "cd $(REMOTE_DIR) && sudo docker-compose up -d"

deploy-restart:
	$(SSH) "cd $(REMOTE_DIR) && sudo docker-compose down && sudo docker-compose up -d"

deploy-prune:
	$(SSH) "docker system prune -a -f && docker volume prune -a -f"

deploy-logs:
	$(SSH) "cd $(REMOTE_DIR) && sudo docker-compose logs -f"

deploy-status:
	$(SSH) "cd $(REMOTE_DIR) && sudo docker-compose ps"

deploy-run-once:
	$(SSH) "cd $(REMOTE_DIR) && sudo docker-compose run --rm trtg"

deploy-clean:
	$(SSH) "cd $(REMOTE_DIR) && sudo docker-compose down -v && rm -rf $(REMOTE_DIR)"

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
	@echo "  docker-build       - Build Docker image for trtg"
	@echo "  docker-build-web   - Build Docker image for trtg-web"
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
	@echo "  deploy-start       - Start services on server"
	@echo "  deploy-stop        - Stop services on server"
	@echo "  deploy-restart     - Restart services on server"
	@echo "  deploy-prune       - Prune Docker system and volumes on server"
	@echo "  deploy-logs        - Show logs from server"
	@echo "  deploy-status      - Show service status"
	@echo "  deploy-run-once    - Run trtg once on server"
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
