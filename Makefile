.PHONY: build run test clean deps lint docker-build docker-build-web docker-run docker-stop deploy deploy-start deploy-stop deploy-restart deploy-prune deploy-backup deploy-backup-list deploy-backup-download deploy-backup-restore help

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
	$(SCP) -r pkg cmd nginx scripts $(REMOTE):$(REMOTE_DIR)/
	$(SCP) prod.env $(REMOTE):$(REMOTE_DIR)/.env
	$(SSH) "cd $(REMOTE_DIR) && sudo mkdir -p backups && sudo chown -R $(REMOTE_USER):$(REMOTE_USER) backups"
	$(SSH) "cd $(REMOTE_DIR) && sudo docker-compose down || true && sudo docker-compose build --no-cache && sudo docker system prune -a -f && sudo docker-compose up -d"

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
	$(SSH) "docker system prune -a -f"

deploy-logs:
	$(SSH) "cd $(REMOTE_DIR) && sudo docker-compose logs -f"

deploy-status:
	$(SSH) "cd $(REMOTE_DIR) && sudo docker-compose ps"

deploy-run-once:
	$(SSH) "cd $(REMOTE_DIR) && sudo docker-compose run --rm trtg"

deploy-clean:
	$(SSH) "cd $(REMOTE_DIR) && sudo docker-compose down -v && rm -rf $(REMOTE_DIR)"

# Database tools
diagnose: ## Run diagnostic tool locally to check parser status
	go run ./cmd/diagnose

reparse-dry-run: ## Preview what would be updated locally (dry run)
	go run ./cmd/reparse -dry-run

reparse: ## Re-parse all videos and update database locally
	go run ./cmd/reparse

deploy-diagnose: ## Run diagnostic tool on remote server
	$(SCP) Dockerfile.diagnose $(REMOTE):$(REMOTE_DIR)/
	$(SCP) -r pkg cmd go.mod go.sum $(REMOTE):$(REMOTE_DIR)/
	$(SSH) "cd $(REMOTE_DIR) && sudo docker build -f Dockerfile.diagnose -t trtg-diagnose . && sudo docker run --rm --network host -e DATABASE_URL='postgres://trtg:trtg@127.0.0.1:5432/trtg?sslmode=disable' trtg-diagnose"

deploy-reparse-dry-run: ## Preview what would be updated on remote server (dry run)
	$(SCP) Dockerfile.reparse $(REMOTE):$(REMOTE_DIR)/
	$(SCP) -r pkg cmd go.mod go.sum $(REMOTE):$(REMOTE_DIR)/
	@$(SSH) "cd $(REMOTE_DIR) && sudo docker build -f Dockerfile.reparse -t trtg-reparse . && ANTHROPIC_API_KEY=\$$(grep ANTHROPIC_API_KEY .env | cut -d= -f2-) && sudo docker run --rm --network host -e DATABASE_URL='postgres://trtg:trtg@127.0.0.1:5432/trtg?sslmode=disable' -e ANTHROPIC_API_KEY=\"\$$ANTHROPIC_API_KEY\" trtg-reparse -dry-run"

deploy-reparse: ## Re-parse all videos and update database on remote server
	$(SCP) Dockerfile.reparse $(REMOTE):$(REMOTE_DIR)/
	$(SCP) -r pkg cmd go.mod go.sum $(REMOTE):$(REMOTE_DIR)/
	@$(SSH) "cd $(REMOTE_DIR) && sudo docker build -f Dockerfile.reparse -t trtg-reparse . && ANTHROPIC_API_KEY=\$$(grep ANTHROPIC_API_KEY .env | cut -d= -f2-) && sudo docker run --rm --network host -e DATABASE_URL='postgres://trtg:trtg@127.0.0.1:5432/trtg?sslmode=disable' -e ANTHROPIC_API_KEY=\"\$$ANTHROPIC_API_KEY\" trtg-reparse"

deploy-reupload: ## Re-upload a broken video by ID (usage: make deploy-reupload VIDEO_ID=848)
	@test -n "$(VIDEO_ID)" || (echo "Error: VIDEO_ID is required. Usage: make deploy-reupload VIDEO_ID=848" && exit 1)
	$(SCP) Dockerfile.reupload $(REMOTE):$(REMOTE_DIR)/
	$(SCP) -r pkg cmd go.mod go.sum $(REMOTE):$(REMOTE_DIR)/
	$(SSH) "cd $(REMOTE_DIR) && sudo docker build -f Dockerfile.reupload -t trtg-reupload . && sudo docker run --rm --network host -v /tmp/reupload:/tmp/downloads -e DATABASE_URL='postgres://trtg:trtg@127.0.0.1:5432/trtg?sslmode=disable' -e TELEGRAM_BOT_TOKEN=\$$(grep TELEGRAM_BOT_TOKEN .env | cut -d= -f2-) -e TELEGRAM_CHAT_ID=\$$(grep TELEGRAM_CHAT_ID .env | cut -d= -f2-) -e TELEGRAM_API_URL='http://localhost:8081' trtg-reupload -video-id $(VIDEO_ID)"

# Database backup tools
deploy-backup: ## Manually trigger database backup on remote server
	$(SSH) "cd $(REMOTE_DIR) && sudo docker exec trtg-postgres-backup /backup.sh"

deploy-backup-list: ## List all database backups on remote server
	$(SSH) "cd $(REMOTE_DIR) && ls -lh backups/trtg_backup_*.sql.gz 2>/dev/null || echo 'No backups found'"

deploy-backup-download: ## Download latest backup from remote server (usage: make deploy-backup-download)
	@echo "Downloading latest backup..."
	@LATEST=$$($(SSH) "cd $(REMOTE_DIR) && ls -t backups/trtg_backup_*.sql.gz 2>/dev/null | head -1"); \
	if [ -z "$$LATEST" ]; then \
		echo "No backups found on remote server"; \
		exit 1; \
	fi; \
	echo "Downloading $$LATEST..."; \
	$(SCP) "$(REMOTE):$(REMOTE_DIR)/$$LATEST" ./backups/

deploy-backup-restore: ## Restore database from backup (usage: make deploy-backup-restore BACKUP_FILE=backups/trtg_backup_20240101_120000.sql.gz)
	@test -n "$(BACKUP_FILE)" || (echo "Error: BACKUP_FILE is required. Usage: make deploy-backup-restore BACKUP_FILE=backups/trtg_backup_20240101_120000.sql.gz" && exit 1)
	@test -f "$(BACKUP_FILE)" || (echo "Error: Backup file $(BACKUP_FILE) not found" && exit 1)
	@echo "WARNING: This will restore the database from $(BACKUP_FILE)"
	@echo "Press Ctrl+C to cancel, or Enter to continue..."
	@read dummy
	$(SCP) "$(BACKUP_FILE)" "$(REMOTE):$(REMOTE_DIR)/backups/restore.sql.gz"
	$(SSH) "cd $(REMOTE_DIR) && gunzip -c backups/restore.sql.gz | sudo docker exec -i trtg-postgres psql -U trtg -d trtg"
	@echo "Database restored successfully"

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
	@echo "Database tools (local):"
	@echo "  diagnose           - Check parser status and show uncategorized videos"
	@echo "  reparse-dry-run    - Preview what would be updated (dry run)"
	@echo "  reparse            - Re-parse all videos and update database"
	@echo ""
	@echo "Database tools (remote):"
	@echo "  deploy-diagnose    - Check parser status on remote server"
	@echo "  deploy-reparse-dry-run - Preview updates on remote server (dry run)"
	@echo "  deploy-reparse     - Re-parse all videos on remote server"
	@echo ""
	@echo "Database backup (remote):"
	@echo "  deploy-backup      - Manually trigger database backup on remote server"
	@echo "  deploy-backup-list - List all database backups on remote server"
	@echo "  deploy-backup-download - Download latest backup from remote server"
	@echo "  deploy-backup-restore - Restore database from backup file"
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
