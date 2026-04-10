# ---------- 构建阶段 ----------
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git make

WORKDIR /app

# 缓存依赖层
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并构建
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/bin/global-sync ./cmd/server

# ---------- 运行阶段 ----------
FROM alpine:3.19

RUN apk add --no-cache ca-certificates curl dumb-init

# 创建非特权用户
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

COPY --from=builder --chown=appuser:appgroup /app/bin/global-sync ./global-sync

USER appuser

ENV APP_ENV=production \
    LOG_FORMAT=json

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["dumb-init", "--"]
CMD ["./global-sync"]
