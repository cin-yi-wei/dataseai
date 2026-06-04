# dataseai

Self-hosted MySQL administration tool with AI chat. Browse, query, edit databases from a browser — with built-in AI assistance via Anthropic Claude, OpenAI, or Google Gemini.

![Go](https://img.shields.io/badge/go-1.25-blue) ![React](https://img.shields.io/badge/react-18-blue) ![License](https://img.shields.io/badge/license-MIT-green)

## Features

- 🗄️ **Browse / Query / Edit** MySQL tables in a web UI
- 🤖 **AI Chat** — talk to your DB via Anthropic, OpenAI, or Gemini (free tier!)
- ⌨️ **SQL Editor** — schema-aware autocomplete, cursor-based statement execution
- 🎯 **Right-click menu** — 15+ actions (copy, paste, set value, quick filter, etc.)
- 🔍 **Multi-condition filter** — 17 operators (LIKE, IN, BETWEEN, IS NULL, ...)
- 🌳 **JSON tree editor** — edit nested JSON with rename + type preservation
- 🌓 **Dark / Light mode** with persistence
- 👥 **Multi-user** with admin panel (stats, users, connections)
- 🔌 **MCP support** — wire any MySQL MCP server (e.g. `@askdba/mcp-server-mysql`)
- 🔐 **AES-GCM** encrypted connection passwords
- 📜 **Query history** per user with replay
- 🎲 **Editable cells** with diff confirmation modal
- 📦 **Import / Export** CSV
- ⚡ **WebSocket** streaming for long queries

## Quick start (Docker Compose)

```bash
cp .env.example .env
# Generate a master key and paste it into .env:
echo "MYSQLWEB_MASTER_KEY=$(openssl rand -hex 32)" >> .env

docker compose up -d --build
open http://localhost:53306    # registration is open by default
```

## Quick start (Go + Node)

```bash
cd web && npm ci && npm run build && cd ..
go build -o ./bin/dataseai ./cmd/dataseai
MYSQLWEB_DB_PATH=./data/dataseai.db ./bin/dataseai
```

## Environment variables

| Variable | Default | Notes |
|---|---|---|
| `MYSQLWEB_PORT` | `53306` | HTTP listen port |
| `MYSQLWEB_DB_PATH` | `/data/dataseai.db` | SQLite location (mount for persistence) |
| `MYSQLWEB_MASTER_KEY` | (auto) | 64-char hex; encrypts stored DB passwords. Generate: `openssl rand -hex 32` |
| `MYSQLWEB_REGISTRATION` | `open` | `open` / `closed` |
| `MYSQLWEB_HISTORY_MAX` | `1000` | Query history cap per user |
| `MYSQLWEB_QUERY_TIMEOUT_S` | `5` | Short-query timeout |
| `MYSQLWEB_QUERY_HTTP_MAX_MB` | `10` | Short-query response cap |
| `MYSQLWEB_LLM_DEFAULT` | `anthropic` | `anthropic` / `openai` / `gemini` |
| `ANTHROPIC_API_KEY` | — | Enables Anthropic Claude |
| `OPENAI_API_KEY` | — | Enables OpenAI GPT |
| `GEMINI_API_KEY` | — | Enables Google Gemini (free tier available) |
| `MYSQLWEB_MCP_COMMAND` | — | Shell command spawning an MCP server. Example: `npx -y @askdba/mcp-server-mysql` |

## AI providers

- **Anthropic** — https://console.anthropic.com/ (new users get $5 free)
- **OpenAI** — https://platform.openai.com/api-keys (pay-as-you-go)
- **Gemini** — https://aistudio.google.com/apikey (**1500 requests/day free**)

Set at least one of the keys above and pick `MYSQLWEB_LLM_DEFAULT` accordingly.

## Admin panel

The first registered user is automatically an admin. Admins can:
- View aggregate stats (users / sessions / queries / connections)
- Promote / demote other users
- Delete users (cascades to their connections + history)
- View all connections across the install

## Tech stack

- **Backend**: Go 1.25, chi router, go-sql-driver/mysql, mattn/go-sqlite3
- **Frontend**: React 18 + TypeScript + Vite, TanStack Table, CodeMirror 6, Zustand
- **Crypto**: AES-GCM via Go stdlib
- **Auth**: bcrypt + opaque session tokens

## License

MIT — see [LICENSE](./LICENSE).
