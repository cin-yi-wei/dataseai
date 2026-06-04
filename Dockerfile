# ---- 1. frontend build ----
FROM node:20-alpine AS frontend
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- 2. go build ----
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache gcc musl-dev
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
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /out/dataseai /usr/local/bin/dataseai
WORKDIR /data
VOLUME ["/data"]
EXPOSE 53306
ENV MYSQLWEB_PORT=53306 \
    MYSQLWEB_DB_PATH=/data/dataseai.db \
    TZ=Asia/Taipei
ENTRYPOINT ["dataseai"]
