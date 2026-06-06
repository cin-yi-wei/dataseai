# Deploying dataseai to a small VM

This folder holds everything needed to run dataseai on a single Linux VM
(tested on a GCP e2-micro, but works on any 1+ GB Linux box) behind a
Cloudflare Tunnel — no open ports, automatic HTTPS, automatic image updates.

## Architecture

```
user ── HTTPS ──> Cloudflare edge ── encrypted tunnel ──> VM
                                                          ├─ dataseai      (image pulled from GHCR)
                                                          ├─ cloudflared   (tunnel agent)
                                                          └─ watchtower    (auto-pulls new image)
```

## One-time setup

### 1. Create a Cloudflare Tunnel

1. Go to https://one.dash.cloudflare.com → your zone → **Networks → Tunnels → Create a tunnel**.
2. Connector type: **Cloudflared**.
3. Name it (e.g. `gcp-dataseai`) and **copy the token** (long `eyJ...` string).
4. Add a Public Hostname:
   - Subdomain: `dataseai`
   - Domain: `conray.top`
   - Type: `HTTP`
   - URL: `dataseai:53306`
5. DNS for `dataseai.conray.top` will be set to a CNAME pointing at the tunnel
   automatically. If the old hostname was pointing at a different tunnel,
   Cloudflare will offer to replace it — accept.

### 2. Bootstrap the VM

SSH into the VM and run:

```bash
curl -fsSL https://raw.githubusercontent.com/cin-yi-wei/dataseai/main/deploy/setup-vm.sh | bash
```

(Or `git clone` the repo and run `deploy/setup-vm.sh` directly.)

This installs Docker, adds 2 GB swap (e2-micro only has 1 GB RAM), and creates
`/opt/dataseai/`.

Log out and back in once (or `newgrp docker`) so your user can run `docker`
without `sudo`.

### 3. Configure and start

```bash
cd /opt/dataseai
curl -fsSL https://raw.githubusercontent.com/cin-yi-wei/dataseai/main/deploy/docker-compose.yml -o docker-compose.yml
curl -fsSL https://raw.githubusercontent.com/cin-yi-wei/dataseai/main/deploy/.env.example -o .env

# Fill in secrets:
#   MYSQLWEB_MASTER_KEY  →  openssl rand -hex 32
#   CLOUDFLARED_TOKEN    →  paste the token from step 1
nano .env

docker compose up -d
docker compose logs -f
```

Once `cloudflared` logs show `Registered tunnel connection`, open
https://dataseai.conray.top — it should serve the app over Cloudflare HTTPS.

## Future deploys (automatic)

Push to `main` → GitHub Actions builds and pushes
`ghcr.io/cin-yi-wei/dataseai:latest` → Watchtower on the VM pulls the new image
within 60 seconds and restarts the container.

No SSH needed for routine updates.

## Useful commands on the VM

```bash
# Watch live logs
docker compose -f /opt/dataseai/docker-compose.yml logs -f dataseai

# Force an immediate update (don't wait for watchtower)
docker compose -f /opt/dataseai/docker-compose.yml pull
docker compose -f /opt/dataseai/docker-compose.yml up -d

# Backup the SQLite DB
cp /opt/dataseai/data/dataseai.db /opt/dataseai/data/dataseai.db.bak.$(date +%F)
```
