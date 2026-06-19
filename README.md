# dataseai

Self-hosted, multi-engine SQL admin tool with an AI chat that can both **read** and (with your explicit approval) **write** to your databases.

![Go](https://img.shields.io/badge/go-1.25-blue) ![React](https://img.shields.io/badge/react-18-blue) ![License](https://img.shields.io/badge/license-MIT-green)

## Supported engines

MySQL · PostgreSQL · SQL Server · SQLite · Oracle · MariaDB · TiDB · PlanetScale · SingleStore · CockroachDB · Redshift · ClickHouse · ByteHouse · Snowflake · DuckDB

Each connection picks its engine; the right dialect is loaded automatically.

## Features

### Browse + edit
- 🗄️ **Table browser** with sortable / filterable rows
- 🔍 **Multi-condition filter** — 17 operators (LIKE, IN, BETWEEN, IS NULL, …) with per-table memory + recent-filter dropdown
- ↔️ **Resizable columns** — drag any column header to set width; long values truncate with an ellipsis + hover tooltip so one big cell never blows out the grid
- ⏹️ **Cancellable browse** — a heavy filtered search can be aborted from the toolbar (cancels the server query too)
- 🔑 **Read-only without a PK** — tables with no primary key are read-only; an edit attempt explains why and offers a ready-to-copy `ALTER … ADD PRIMARY KEY`
- 🎯 **Right-click / long-press menu** — 15+ actions (copy, paste, set value, quick filter, …)
- 🌳 **JSON tree editor** — edit nested JSON with rename + type preservation
- ✏️ **Editable cells** with diff confirmation modal
- 📦 **CSV import / export**

### SQL workspace
- ⌨️ **SQL editor** with schema-aware autocomplete + cursor-based statement execution
- ⚡ **WebSocket streaming** for long queries (cancellable)
- 📊 **Resizable result columns** with truncation + click-to-expand for long values
- 📜 **Query history** per user with replay
- 🧷 **Persistent tabs** — open tabs survive reloads, one click to clear all
- 🔌 **SSH tunnel** per connection (dial the DB through a bastion host)
- 🧰 **Resizable sidebar** — drag the divider to widen the table list for long names

### AI chat
- 🤖 **Five providers**:
  - **API key**: Anthropic Claude · OpenAI · Google Gemini (free tier 20/day)
  - **OAuth (rides your subscription)**: Claude Code (Pro/Max) · ChatGPT (Plus/Pro)
- 🛡️ **AI write permissions** — per-user opt-in policy gating `INSERT / UPDATE / DELETE / DDL` per (connection, db, table). Every write surfaces as a preview card; you click Execute before it runs. Full audit log.
- 🔧 **Tool use** — the model can call `list_databases`, `query_table`, `describe_table`, `run_sql` (read-only), and `propose_write` against your live database.

### Local connector (agent)
- 🌉 **Reach LAN / localhost / bastion-only databases** — run the separate [`dataseai-connector`](https://github.com/cin-yi-wei/dataseai-connector) on a machine inside the target network. It dials **out** to dataseai over WebSocket, so a cloud-hosted dataseai can query databases it could never reach directly (home/office LAN, `localhost:3306`, VPN/SSH-bastion-only hosts) with no inbound ports opened.
- 🔐 Per-agent token auth; a connection is bound to an agent via `via_agent_id`. The connector can also open an SSH tunnel to the final database.

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
| `MYSQLWEB_QUERY_TIMEOUT_S` | `5` | SQL-editor short-query timeout; longer queries fall through to cancellable WebSocket streaming |
| `MYSQLWEB_QUERY_HTTP_MAX_MB` | `10` | Short-query response cap |
| `MYSQLWEB_LLM_DEFAULT` | `anthropic` | `anthropic` / `openai` / `gemini` / `claudecode` / `codex` |
| `ANTHROPIC_API_KEY` | — | Server-wide fallback (per-user keys take precedence) |
| `OPENAI_API_KEY` | — | Same |
| `GEMINI_API_KEY` | — | Same |

API keys are usually configured **per-user via Settings**, not via env. Env vars are only the server-wide fallback.

> Read/browse operations (list databases & tables, schema, table data + filter search, structure, indexes, FKs) use a generous 900s server timeout so heavy filtered searches over large tables don't abort early. They're cancellable from the UI.

## Admin panel

The first registered user is automatically an admin. Admins can:
- View aggregate stats (users / sessions / queries / connections)
- Promote / demote other users
- Delete users (cascades to their connections + history)
- View all connections across the install

## Architecture

- **Backend** — Go 1.25, chi router, `coder/websocket`, per-engine drivers (`go-sql-driver/mysql`, `jackc/pgx`, `microsoft/go-mssqldb`, …) behind a pluggable dialect registry
- **Frontend** — React 18 + TypeScript + Vite, TanStack Table, CodeMirror 6, Zustand, react-markdown
- **Storage** — SQLite for app metadata (users, sessions, connections, AI policy + audit, OAuth tokens); the target databases are never touched for app state
- **Connector** — a `/agent` WebSocket broker registers outbound connectors; via-agent connections route queries through them to reach otherwise-unreachable networks
- **Auth** — bcrypt password hashing + opaque session tokens
- **Crypto** — AES-GCM via Go stdlib for connection passwords, LLM API keys, and OAuth tokens

## License

MIT — see [LICENSE](./LICENSE).
