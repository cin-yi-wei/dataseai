# dataseai dev/deploy helpers
#
#   make dev        — rebuild + restart the local dataseai (fast iteration, served at dataseai-test.conray.top)
#   make build      — rebuild frontend + Go binary only
#   make stop       — stop the local dataseai
#   make logs       — tail local server log
#   make deploy     — push main; GHCR build + Watchtower roll out to GCP (3–5 min)
#   make ssh-prod   — SSH to the GCP VM
#   make logs-prod  — tail GCP dataseai container logs
#   make redeploy-prod — force VM to pull :latest now (don't wait for Watchtower)

PROJECT_DIR := $(CURDIR)
DB_PATH     := $(PROJECT_DIR)/data/dataseai.db
PORT        := 53306
LOG_FILE    := $(PROJECT_DIR)/logs/mysqlweb.log
BIN         := $(PROJECT_DIR)/bin/dataseai

VM_USER     := conray_nas
VM_HOST     := 136.118.1.198
VM_APP_DIR  := /opt/dataseai

LOCAL_URL   := https://dataseai-test.conray.top
PROD_URL    := https://dataseai.conray.top

.PHONY: build stop dev logs deploy ssh-prod logs-prod redeploy-prod status

build:
	cd $(PROJECT_DIR)/web && npm run build
	cd $(PROJECT_DIR) && go build -o $(BIN) ./cmd/dataseai

stop:
	-pkill -f '$(BIN)$$' 2>/dev/null
	@sleep 1

dev: stop build
	@mkdir -p $(PROJECT_DIR)/logs
	cd $(PROJECT_DIR) && setsid env MYSQLWEB_DB_PATH=$(DB_PATH) MYSQLWEB_PORT=$(PORT) \
		$(BIN) > $(LOG_FILE) 2>&1 < /dev/null & disown
	@sleep 2
	@curl -fs http://127.0.0.1:$(PORT)/api/health && echo "  ← local OK" || echo "  ← local not responding"
	@echo "→ $(LOCAL_URL)"

logs:
	tail -f $(LOG_FILE)

deploy:
	@git -C $(PROJECT_DIR) status --short
	@echo "→ pushing main; CI will build + push image, watchtower picks it up in ~60s after CI completes"
	git -C $(PROJECT_DIR) push origin main
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
