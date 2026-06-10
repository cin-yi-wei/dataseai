# dataseai dev/deploy helpers
#
#   make dev        — rebuild + restart the local dataseai (fast iteration, served at dataseai-test.conray.top)
#   make build      — rebuild frontend + Go binary only
#   make stop       — stop the local dataseai
#   make logs       — tail local server log
#   make push       — push current branch (no deploy if it's dev)
#   make deploy     — fast-forward main to dev's HEAD and push; triggers GHCR build + Watchtower roll (3–5 min)
#   make ssh-prod   — SSH to the GCP VM
#   make logs-prod  — tail GCP dataseai container logs
#   make redeploy-prod — force VM to pull :latest now (don't wait for Watchtower)

PROJECT_DIR := $(CURDIR)
DB_PATH     := $(PROJECT_DIR)/data/dataseai.db
PORT        := 53306
LOG_FILE    := $(PROJECT_DIR)/logs/mysqlweb.log
PID_FILE    := $(PROJECT_DIR)/.dataseai.pid
BIN         := $(PROJECT_DIR)/bin/dataseai

VM_USER     := conray_nas
VM_HOST     := 136.118.1.198
VM_APP_DIR  := /opt/dataseai

LOCAL_URL   := https://dataseai-test.conray.top
PROD_URL    := https://dataseai.conray.top

.PHONY: build stop dev logs push deploy ssh-prod logs-prod redeploy-prod status

build:
	cd $(PROJECT_DIR)/web && npm run build
	cd $(PROJECT_DIR) && go build -o $(BIN) ./cmd/dataseai

stop:
	-pkill -f '($(BIN)|\./bin/dataseai)$$' 2>/dev/null
	@sleep 1

dev: stop build
	@mkdir -p $(PROJECT_DIR)/logs
	cd $(PROJECT_DIR) && setsid env MYSQLWEB_DB_PATH=$(DB_PATH) MYSQLWEB_PORT=$(PORT) \
		$(BIN) > $(LOG_FILE) 2>&1 < /dev/null & echo $$! > $(PID_FILE)
	@sleep 2
	@curl -fs http://127.0.0.1:$(PORT)/api/health && echo "  ← local OK" || echo "  ← local not responding"
	@echo "→ $(LOCAL_URL)"

logs:
	tail -f $(LOG_FILE)

push:
	@git -C $(PROJECT_DIR) status --short
	git -C $(PROJECT_DIR) push origin HEAD

deploy:
	@cd $(PROJECT_DIR) && git status --short
	@echo "→ fast-forwarding remote main to dev's HEAD"
	cd $(PROJECT_DIR) && git push origin dev:main
	cd $(PROJECT_DIR) && git fetch origin && git branch -f main origin/main
	@echo "→ CI will build + push image to GHCR; watchtower picks it up in ~60s after CI completes"
	@echo "Watch: https://github.com/cin-yi-wei/dataseai/actions"

ssh-prod:
	ssh $(VM_USER)@$(VM_HOST)

logs-prod:
	ssh $(VM_USER)@$(VM_HOST) 'sudo docker logs -f --tail 50 dataseai-dataseai-1'

redeploy-prod:
	ssh $(VM_USER)@$(VM_HOST) 'cd $(VM_APP_DIR) && sudo docker compose pull && sudo docker compose up -d && sudo docker compose logs --tail 20 dataseai'

status:
	@echo "=== local ==="
	@curl -fs http://127.0.0.1:$(PORT)/api/health || echo "  (down)"
	@echo
	@echo "=== $(LOCAL_URL) ==="
	@curl -fs $(LOCAL_URL)/api/health || echo "  (down)"
	@echo
	@echo "=== $(PROD_URL) ==="
	@curl -fs $(PROD_URL)/api/health || echo "  (down)"
	@echo
