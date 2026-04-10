# Global Sync Service - 项目计划

> **服务名称**: pv-global-sync-service
> **技术栈**: Go + RocketMQ + Redis Stack + PostgreSQL
> **用途**: WigoWago V2 分布式架构的跨区域同步、全局索引和 Feed 生成服务
> **创建日期**: 2026-04-10
> **当前状态**: 项目初始化 (Phase 0)

---

## 一、项目背景与目标

### 1.1 背景

WigoWago 当前是单体 Node.js 应用，部署在北美 Azure (eastus2)。为满足 GDPR 合规要求，需要在欧洲 (West Europe) 部署第二套集群。在数据本地化的前提下，实现跨区域公开内容共享。

### 1.2 服务职责

Global Sync Service 是独立于 WigoWago API 的 Go 语言微服务，负责：

1. **跨区域同步**: 接收 WigoWago API 发布的事件，同步公开内容到对端区域
2. **全局索引**: 维护 global_post_index 表，提供跨区域内容检索能力
3. **Feed 生成**: Push/Pull 混合模式生成用户 Feed (Following / Global / Trending)
4. **GDPR 合规**: 同步规则引擎、审计日志、用户同意验证

### 1.3 设计原则

- **独立部署**: 与 WigoWago API 完全解耦，独立扩缩容
- **事件驱动**: 通过 RocketMQ 消费 API 发布的同步事件
- **最终一致性**: 跨区域同步采用异步最终一致性
- **合规优先**: 所有同步操作经过 GDPR 规则引擎检查

---

## 二、技术架构

### 2.1 技术选型

| 组件 | 选型 | 理由 |
|------|------|------|
| **语言** | Go 1.22+ | 高并发、低内存、团队学习成本低于 Rust |
| **消息队列** | Apache RocketMQ | 基础设施已有，无需新增中间件成本 |
| **数据库** | PostgreSQL | 区域 DB + Global Index DB 双实例 |
| **缓存** | Redis Stack | JSON 原生支持 + RediSearch + 时间序列 |
| **HTTP 框架** | 标准库 net/http + chi router | 轻量、零依赖 |
| **ORM** | sqlc 或手动 SQL | 性能可控，避免 ORM 黑盒 |
| **日志** | zap | 结构化日志、高性能 |

### 2.2 服务结构

```
global-sync-service/
├── cmd/server/main.go              # 服务入口
├── internal/
│   ├── config/config.go            # 配置管理 (环境变量)
│   ├── server/server.go            # HTTP Server (chi router)
│   ├── health/health.go            # 健康检查
│   ├── middleware/                  # HTTP 中间件
│   │   ├── request_logger.go
│   │   ├── recovery.go
│   │   └── metrics.go
│   ├── model/
│   │   └── sync_event.go           # 同步事件数据模型
│   ├── consumer/
│   │   └── sync_consumer.go        # RocketMQ 消费者 (本地事件)
│   ├── producer/
│   │   └── sync_producer.go        # RocketMQ 生产者 (跨区域发送)
│   ├── handler/
│   │   ├── sync_handler.go         # HTTP 同步接口处理
│   │   ├── feed_handler.go         # Feed 生成与查询
│   │   └── gdpr_handler.go         # GDPR 合规检查
│   └── service/
│       ├── global_index.go         # Global Index DB 操作
│       ├── feed_generator.go       # Feed 生成器 (Push/Pull)
│       ├── cross_sync.go           # 跨区域同步协调
│       └── audit_log.go            # GDPR 审计日志
├── pkg/
│   ├── rocketmq/                   # RocketMQ 客户端封装
│   ├── postgres/                   # PostgreSQL 连接池封装
│   ├── redis/                      # Redis 客户端封装
│   ├── logger/                     # zap 日志封装
│   └── snowflake/                  # Snowflake ID 生成
├── deployments/
│   ├── helm/global-sync/           # Helm Chart
│   └── k8s/                        # K8s manifests (过渡用)
├── docs/                           # 项目文档
│   └── PROJECT_PLAN.md             # 本文件
├── scripts/                        # 运维脚本
├── test/                           # 集成测试
├── go.mod
└── go.sum
```

### 2.3 数据流

```
WigoWago API (Node.js)
    │
    ├─→ 写入 Regional DB (PostgreSQL)
    │
    └─→ 发布事件到 RocketMQ (本地集群)
            │
            ▼
    Global Sync Service (Go)
            │
            ├─→ Consumer: 消费本地 RocketMQ 事件
            │       │
            │       ├─→ GDPR 规则引擎检查
            │       │       ├─ 拒绝 (Tier 1 PII, 私密内容)
            │       │       └─ 通过 (公开内容)
            │       │
            │       ├─→ 写入 Global Index DB (PostgreSQL)
            │       ├─→ 更新 Redis 缓存
            │       └─→ 触发 Feed 生成
            │
            └─→ Producer: 发送到对端区域 RocketMQ
                    │
                    ▼
            对端 Global Sync Service
                    │
                    └─→ 写入对端 Global Index DB
```

---

## 三、实施计划

### Phase 0: 项目初始化 (Week 1) ✅ 当前阶段

**目标**: 建立项目骨架、CI/CD 基础、开发环境

| 任务 | 状态 | 说明 |
|------|------|------|
| 创建 GitHub 仓库 | ✅ | petaverse-cloud/pv-global-sync-service |
| 初始化 Go Module | ✅ | go.mod 创建 |
| 项目目录结构 | ✅ | cmd/internal/pkg 分层 |
| 基础骨架代码 | ✅ | main.go, config, logger, server, health |
| .gitignore + Dockerfile 模板 | ⬜ | 基础构建配置 |
| Makefile | ⬜ | 常用命令 (build, test, lint) |
| .golangci.yml | ⬜ | Go 代码规范检查 |
| GitHub Actions CI | ⬜ | lint + test + build |

### Phase 1: 核心基础设施 (Week 2-3)

**目标**: 数据库连接、Redis 连接、RocketMQ 集成

| 任务 | 优先级 | 说明 |
|------|--------|------|
| PostgreSQL 封装 | P0 | 双数据源 (Regional + Global Index) |
| Redis 封装 | P0 | 连接池 + 常用操作封装 |
| RocketMQ 封装 | P0 | Producer + Consumer 封装 |
| SQL Schema 定义 | P0 | Global Index + Feed + Audit Log 表 |
| 配置验证 | P1 | 启动时检查必需配置 |
| 健康检查完善 | P1 | DB + Redis + RocketMQ 连通性 |

### Phase 2: 同步引擎 (Week 4-5)

**目标**: 实现核心同步逻辑

| 任务 | 优先级 | 说明 |
|------|--------|------|
| 同步事件消费 | P0 | RocketMQ Consumer 消费 API 事件 |
| GDPR 规则引擎 | P0 | 数据分类、可见性、用户同意检查 |
| Global Index 写入 | P0 | global_post_index 表 CRUD |
| 跨区域同步 | P0 | HTTP POST 到对端区域 /sync/cross-sync |
| 冲突解决策略 | P1 | Last-Write-Wins、删除优先 |
| 重试与幂等 | P1 | 事件去重、失败重试 |

### Phase 3: Feed 生成 (Week 6-7)

**目标**: 实现 Push/Pull 混合 Feed

| 任务 | 优先级 | 说明 |
|------|--------|------|
| Push 模式 (Fan-out) | P0 | 普通用户发布时预计算到粉丝 Feed |
| Pull 模式 (Fan-in) | P0 | 名人用户读取时实时聚合 |
| Feed 排序算法 | P1 | 时间衰减 + 互动率 + 亲密度 + 偏好 |
| Redis Feed 缓存 | P0 | ZSET 存储 + TTL 管理 |
| Feed API | P0 | GET /feed/:userId?feedType=... |
| 缓存刷新策略 | P1 | 新帖发布时 invalidate 相关缓存 |

### Phase 4: GDPR 合规 (Week 8)

**目标**: 完善合规能力

| 任务 | 优先级 | 说明 |
|------|--------|------|
| 审计日志 | P0 | 所有跨境传输记录到 audit_log 表 |
| 删除权实现 | P0 | 接收删除事件后从 Global Index 移除 |
| 用户同意管理 | P1 | 检查 cross_border_transfer_allowed 字段 |
| 合规检查清单 | P1 | 自动化验证脚本 |

### Phase 5: 部署与运维 (Week 9-10)

**目标**: 生产就绪

| 任务 | 优先级 | 说明 |
|------|--------|------|
| Dockerfile | P0 | 多阶段构建 |
| Helm Chart | P0 | K8s 部署配置 |
| Prometheus 指标 | P1 | 同步延迟、成功率、Feed 延迟 |
| 日志聚合 | P1 | 结构化日志输出 |
| 集成测试 | P1 | Mock RocketMQ + DB + Redis |
| 压力测试 | P2 | 吞吐量与延迟基准 |

---

## 四、数据库 Schema (Phase 1 定义)

### 4.1 Global Post Index

```sql
CREATE TABLE global_post_index (
    post_id BIGINT PRIMARY KEY,
    author_id BIGINT NOT NULL,
    author_region VARCHAR(2) NOT NULL,
    content_preview TEXT,
    visibility VARCHAR(20) NOT NULL,
    hashtags TEXT[],
    mentions BIGINT[],
    
    -- Stats
    likes_count INTEGER DEFAULT 0,
    comments_count INTEGER DEFAULT 0,
    shares_count INTEGER DEFAULT 0,
    views_count INTEGER DEFAULT 0,
    
    -- Compliance
    gdpr_compliant BOOLEAN NOT NULL DEFAULT false,
    user_consent BOOLEAN NOT NULL DEFAULT false,
    data_category VARCHAR(20) NOT NULL,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL,
    synced_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_gpi_author ON global_post_index(author_id);
CREATE INDEX idx_gpi_visibility ON global_post_index(visibility);
CREATE INDEX idx_gpi_hashtags ON global_post_index USING GIN(hashtags);
CREATE INDEX idx_gpi_created ON global_post_index(created_at DESC);
```

### 4.2 User Feed

```sql
CREATE TABLE user_feed (
    user_id BIGINT NOT NULL,
    post_id BIGINT NOT NULL,
    feed_type VARCHAR(20) NOT NULL,  -- 'following' | 'global' | 'trending'
    score DECIMAL(10,6) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ,
    
    PRIMARY KEY (user_id, feed_type, post_id)
);

CREATE INDEX idx_uf_user ON user_feed(user_id, feed_type, score DESC);
```

### 4.3 Cross-Border Audit Log

```sql
CREATE TABLE cross_border_audit_log (
    log_id BIGSERIAL PRIMARY KEY,
    event_id VARCHAR(64) NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    data_subject_id BIGINT NOT NULL,
    source_region VARCHAR(2) NOT NULL,
    target_region VARCHAR(2) NOT NULL,
    data_type VARCHAR(50) NOT NULL,
    legal_basis VARCHAR(100),
    user_consent BOOLEAN DEFAULT false,
    status VARCHAR(20) NOT NULL,
    metadata JSONB
);

CREATE INDEX idx_audit_timestamp ON cross_border_audit_log(timestamp);
CREATE INDEX idx_audit_subject ON cross_border_audit_log(data_subject_id);
```

### 4.4 Sync Event Log (幂等保证)

```sql
CREATE TABLE sync_event_log (
    event_id VARCHAR(64) PRIMARY KEY,
    event_type VARCHAR(32) NOT NULL,
    source_region VARCHAR(2) NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    error_message TEXT
);
```

---

## 五、Redis 数据结构

```
# User Feed (ZSET, Push 模式)
user:feed:{user_id}:following  → ZSET (post_id, score)
user:feed:{user_id}:global     → ZSET (post_id, score)
user:feed:{user_id}:trending   → ZSET (post_id, score)

# Post Detail Cache (HASH)
post:{post_id}  → HASH (post_data JSON)

# Stats Cache (HASH)
post:{post_id}:stats  → HASH (likes, comments, shares, views)

# Event Deduplication (SET with TTL)
sync:event:processed  → SET (event_ids, TTL 24h)

# GDPR User Consent Cache
user:consent:{user_id}  → STRING (JSON consent data)

# TTL 设置
- Following Feed: 5 分钟
- Global Feed: 15 分钟
- Trending Feed: 1 分钟
- Post Detail: 24 小时
- Event Dedup: 24 小时
```

---

## 六、API 设计 (Phase 2-3)

### 6.1 同步接口 (WigoWago API → Global Sync)

```
POST /sync/content
Content-Type: application/json
Authorization: Bearer <internal-token>

{
  "eventId": "uuid",
  "eventType": "POST_CREATED",
  "sourceRegion": "EU",
  "targetRegion": "NA",
  "timestamp": 1712736000,
  "payload": {
    "postId": 12345,
    "authorId": 678,
    "authorRegion": "EU",
    "visibility": "GLOBAL",
    "content": "Hello world #petlover",
    "mediaUrls": ["https://..."]
  },
  "metadata": {
    "gdprCompliant": true,
    "userConsent": true,
    "dataCategory": "TIER_2"
  }
}

Response: 202 Accepted
```

### 6.2 跨区域同步 (EU Sync → NA Sync)

```
POST /sync/cross-sync
Content-Type: application/json

# 同 /sync/content 格式，sourceRegion 为对端

Response: 202 Accepted
```

### 6.3 Feed 查询

```
GET /feed/:userId?feedType=following&limit=20&cursor=xxx

Response:
{
  "items": [
    {
      "post": {...},
      "author": {...},
      "engagement": {"likes": 42, "comments": 5, "shares": 2},
      "score": 0.85
    }
  ],
  "nextCursor": "eyJpZCI6MTIzfQ==",
  "hasMore": true
}
```

### 6.4 健康检查

```
GET /health        → 200 {"status": "ok", "timestamp": "..."}
GET /health/live   → 200 {"status": "alive"}
GET /health/ready  → 200 {"status": "ready"} (检查 DB/Redis/RocketMQ)
```

---

## 七、RocketMQ 设计

### 7.1 Topic 设计

| Topic | 用途 | 消费者 |
|-------|------|--------|
| `sync-events-{region}` | 区域内同步事件 (WigoWago API 发布) | Global Sync Consumer |
| `cross-sync-eu-to-na` | EU → NA 跨区域同步 | NA Sync Consumer |
| `cross-sync-na-to-eu` | NA → EU 跨区域同步 | EU Sync Consumer |

### 7.2 消息格式

```json
{
  "eventId": "evt_1234567890abcdef",
  "eventType": "POST_CREATED",
  "sourceRegion": "EU",
  "payload": {
    "postId": 12345,
    "authorId": 678,
    "authorRegion": "EU",
    "visibility": "GLOBAL",
    "content": "..."
  },
  "metadata": {
    "gdprCompliant": true,
    "userConsent": true,
    "dataCategory": "TIER_2"
  }
}
```

### 7.3 消费保证

- **顺序消费**: 同一 postId 的事件保证顺序 (MessageKey = postId)
- **至少一次投递**: 消费失败自动重试
- **幂等处理**: 通过 sync_event_log 表去重

---

## 八、风险与缓解

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|----------|
| Go 语言学习曲线 | 开发效率 | 中 | 从简单模块开始，Code Review 把关 |
| RocketMQ 跨区延迟 | 同步延迟 | 中 | 异步处理 + UI 提示 + 重试机制 |
| Redis Stack 部署复杂度 | 运维成本 | 低 | 使用 Azure Redis Enterprise (已支持 RediSearch) |
| 数据不一致 | 用户体验 | 低 | 定期校验 + 冲突解决策略 |
| GDPR 合规风险 | 法律处罚 | 低 | 法律顾问审查 + 完整审计日志 |

---

## 九、与 V2 架构文档的对齐

本文档基于 `wigowago-v2-distributed-architecture.md` 设计，以下关键决策保持一致:

| 决策 | 来源 | 实现 |
|------|------|------|
| Go + RocketMQ 技术栈 | V2 架构文档 3.2 节 | 本项目采用 |
| Push/Pull 混合 Feed | V2 架构文档 5.1 节 | Phase 3 实现 |
| 1000 粉丝阈值 | V2 架构文档 5.1 节 | FeedPushThreshold 配置项 |
| 排序权重 (30/30/30/10) | V2 架构文档 5.3 节 | feed_generator.go 实现 |
| GDPR Tier 分类 | V2 架构文档 2.1 节 | DataCategory 枚举 + 规则引擎 |
| Global Index DB 结构 | V2 架构文档 5.1 节 | Phase 1 Schema 定义 |
| 跨区域同步 HTTP | V2 架构文档 6.1 节 | /sync/cross-sync 端点 |
| LWW 冲突解决 | V2 架构文档 6.4 节 | 基于时间戳 |

---

## 十、下一步 (Phase 0 剩余工作)

1. [ ] 创建 Dockerfile (多阶段构建)
2. [ ] 创建 Makefile (build/test/lint/run)
3. [ ] 配置 .golangci.yml (Go 代码规范)
4. [ ] 创建 GitHub Actions CI (lint + test)
5. [ ] 编写 sqlc 配置或手动 SQL 查询文件
6. [ ] 编写 README.md (项目说明)
7. [ ] 初始提交并推送
