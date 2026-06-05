# dataseai

Self-hosted MySQL admin tool with an AI chat that can both **read** and (with your explicit approval) **write** to your databases.

![Go](https://img.shields.io/badge/go-1.25-blue) ![React](https://img.shields.io/badge/react-18-blue) ![License](https://img.shields.io/badge/license-MIT-green)

## Features

### Browse + edit
- 🗄️ **Table browser** with sortable / filterable rows
- 🔍 **Multi-condition filter** — 17 operators (LIKE, IN, BETWEEN, IS NULL, …) with per-table memory + recent-filter dropdown
- 🎯 **Right-click / long-press menu** — 15+ actions (copy, paste, set value, quick filter, …)
- 🌳 **JSON tree editor** — edit nested JSON with rename + type preservation
- ✏️ **Editable cells** with diff confirmation modal
- 📦 **CSV import / export**

### SQL workspace
- ⌨️ **SQL editor** with schema-aware autocomplete + cursor-based statement execution
- ⚡ **WebSocket streaming** for long queries (cancellable)
- 📜 **Query history** per user with replay
- 🧷 **Persistent tabs** — open tabs survive reloads, one click to clear all
- 🔌 **SSH tunnel** for MySQL connections (per connection)

### AI chat
- 🤖 **Five providers**:
  - **API key**: Anthropic Claude · OpenAI · Google Gemini (free tier 20/day)
  - **OAuth (rides your subscription)**: Claude Code (Pro/Max) · ChatGPT (Plus/Pro)
- 🛡️ **AI write permissions** — per-user opt-in policy gating `INSERT / UPDATE / DELETE / DDL` per (connection, db, table). Every write surfaces as a preview card; you click Execute before it runs. Full audit log.
- 🔧 **Tool use** — the model can call `list_databases`, `query_table`, `describe_table`, `run_sql` (read-only), and `propose_write` against your live MySQL.

### Platform
- 🌓 **Dark / Light mode** with persistence (native scrollbars + form controls follow)
- 🌐 **i18n** — English + 繁體中文 (auto-detected from browser)
- 👥 **Multi-user** with admin panel (stats, users, connections)
- 🔐 **AES-GCM** encrypts connection passwords + OAuth tokens at rest
- 📱 **Mobile-friendly** — responsive layout, touch-optimized DataGrid + tabs

## Quick start (Go + Node)

```bash
cd web && npm ci && npm run build && cd ..
go build -o ./bin/dataseai ./cmd/dataseai

mkdir -p data
MYSQLWEB_DB_PATH=./data/dataseai.db \
  MYSQLWEB_MASTER_KEY=$(openssl rand -hex 32) \
  ./bin/dataseai
```

Open <http://localhost:53306>. The first user to register becomes an admin.

## Quick start (Docker)

```bash
docker build -t dataseai:dev .
docker run -d --name dataseai -p 53306:53306 \
  -v $PWD/data:/data \
  -e MYSQLWEB_MASTER_KEY=$(openssl rand -hex 32) \
  dataseai:dev
```

## AI providers

You can mix and match — each user picks per chat:

| Provider | How to enable | Billing |
|---|---|---|
| **Google Gemini** | Settings → API keys → paste from <https://aistudio.google.com/apikey> | Free tier 20–250 RPD |
| **Anthropic Claude** | Settings → API keys → paste from <https://console.anthropic.com/> | Pay-per-token |
| **OpenAI** | Settings → API keys → paste from <https://platform.openai.com/api-keys> | Pay-per-token |
| **Claude Code (subscription)** | Settings → API keys → 🔗 Connect Claude | Your Claude Pro / Max / Team plan |
| **ChatGPT (subscription)** | Settings → API keys → 🔗 Connect ChatGPT | Your ChatGPT Plus / Pro plan |

The subscription-OAuth flows ride your existing Claude Code / Codex CLI client credentials. Tokens refresh automatically. **Personal / small-team use only — those OAuth clients aren't officially open to third-party apps.**

## Environment variables

| Variable | Default | Notes |
|---|---|---|
| `MYSQLWEB_PORT` | `53306` | HTTP listen port |
| `MYSQLWEB_DB_PATH` | `/data/dataseai.db` | SQLite location (mount for persistence) |
| `MYSQLWEB_MASTER_KEY` | (auto-generated) | 64-char hex; encrypts stored secrets. Generate: `openssl rand -hex 32` |
| `MYSQLWEB_REGISTRATION` | `open` | `open` / `closed` |
| `MYSQLWEB_HISTORY_MAX` | `1000` | Query history cap per user |
| `MYSQLWEB_QUERY_TIMEOUT_S` | `5` | Short-query timeout |
| `MYSQLWEB_QUERY_HTTP_MAX_MB` | `10` | Short-query response cap |
| `MYSQLWEB_LLM_DEFAULT` | `anthropic` | `anthropic` / `openai` / `gemini` / `claudecode` / `codex` |
| `ANTHROPIC_API_KEY` | — | Server-wide fallback (per-user keys take precedence) |
| `OPENAI_API_KEY` | — | Same |
| `GEMINI_API_KEY` | — | Same |

API keys are usually configured **per-user via Settings**, not via env. Env vars are only the server-wide fallback.

## Admin panel

The first registered user is automatically an admin. Admins can:
- View aggregate stats (users / sessions / queries / connections)
- Promote / demote other users
- Delete users (cascades to their connections + history)
- View all connections across the install

## Architecture

- **Backend** — Go 1.25, chi router, `go-sql-driver/mysql`, `mattn/go-sqlite3`, `coder/websocket`
- **Frontend** — React 18 + TypeScript + Vite, TanStack Table, CodeMirror 6, Zustand, react-markdown
- **Storage** — SQLite for app metadata (users, sessions, connections, AI policy + audit, OAuth tokens), MySQL is the target — never touched for app state
- **Auth** — bcrypt password hashing + opaque session tokens
- **Crypto** — AES-GCM via Go stdlib for connection passwords, LLM API keys, and OAuth tokens

## License

MIT — see [LICENSE](./LICENSE).
