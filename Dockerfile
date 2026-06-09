# ---- 1. frontend build ----
FROM node:20-alpine AS frontend
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- 2. go build ----
# golang:1.25 is Debian-based (glibc); go-duckdb ships a pre-built libduckdb.a
# compiled against glibc/libstdc++ which is incompatible with Alpine musl.
FROM golang:1.25 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /web/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/dataseai ./cmd/dataseai

# ---- 3. final ----
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata && rm -rf /var/lib/apt/lists/*
COPY --from=builder /out/dataseai /usr/local/bin/dataseai
WORKDIR /data
VOLUME ["/data"]
EXPOSE 53306
ENV MYSQLWEB_PORT=53306 \
    MYSQLWEB_DB_PATH=/data/dataseai.db \
    TZ=Asia/Taipei
ENTRYPOINT ["dataseai"]
