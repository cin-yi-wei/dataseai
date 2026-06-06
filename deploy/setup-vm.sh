#!/bin/bash
# One-shot VM bootstrap for dataseai on a small Linux VM (e.g. GCP e2-micro).
# Idempotent: safe to re-run.
set -euo pipefail

APP_DIR="${APP_DIR:-/opt/dataseai}"
SWAP_SIZE="${SWAP_SIZE:-2G}"

echo ">>> apt update + base tools"
sudo apt-get update -y
sudo apt-get install -y curl ca-certificates gnupg lsb-release jq

echo ">>> install Docker (if missing)"
if ! command -v docker >/dev/null 2>&1; then
  curl -fsSL https://get.docker.com | sudo sh
  sudo usermod -aG docker "$USER"
fi
sudo systemctl enable --now docker

echo ">>> setup ${SWAP_SIZE} swap (e2-micro only has 1GB RAM)"
if [ ! -f /swapfile ]; then
  sudo fallocate -l "$SWAP_SIZE" /swapfile
  sudo chmod 600 /swapfile
  sudo mkswap /swapfile
  sudo swapon /swapfile
  if ! grep -q '^/swapfile' /etc/fstab; then
    echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
  fi
fi

echo ">>> create app directory at ${APP_DIR}"
sudo mkdir -p "${APP_DIR}/data"
sudo chown -R "$USER:$USER" "${APP_DIR}"

echo ""
echo "=========================================="
echo "Setup complete."
echo ""
echo "Next steps:"
echo "  1. log out and back in (or run: newgrp docker) so docker works without sudo"
echo "  2. drop docker-compose.yml and .env into ${APP_DIR}/"
echo "  3. cd ${APP_DIR} && docker compose up -d"
echo "=========================================="
