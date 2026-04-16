# WigoWago V2 - 架构调整建议

> **基于 Twitter/X 架构演进经验**  
> **创建日期**: 2026-04-08  
> **目标**: 轻量、高可用、易扩展

---

## 📊 Twitter/X 架构演进历程

### 阶段 1: Ruby on Rails 单体 (2006-2010)

```
┌─────────────────────────────────────┐
│         Monolithic Rails App        │
│  - Single Database                  │
│  - All features in one codebase     │
│  - Simple deployment                │
└─────────────────────────────────────┘
```

**问题**:
- ❌ 无法水平扩展
- ❌ 单点故障
- ❌ 开发速度下降

### 阶段 2: 服务拆分 (2010-2015)

```
┌─────────────────────────────────────────────────────┐
│              API Gateway                            │
└─────────────────────────────────────────────────────┘
              │         │         │
              ▼         ▼         ▼
    ┌─────────────┐ ┌─────────┐ ┌──────────┐
    │ Tweet Svc   │ │ User Svc│ │ Timeline │
    └─────────────┘ └─────────┘ └──────────┘
```

**问题**:
- ❌ 过度拆分 (100+ 微服务)
- ❌ 运维复杂度高
- ❌ 服务间依赖混乱

### 阶段 3: 重新整合 (2015-2020)

- 合并相关服务
- 引入 Service Mesh
- 统一监控和日志

### 阶段 4: 云原生 (2020-现在)

- Kubernetes 容器化
- 事件驱动架构
- AI/ML 集成

---

## 🎯 关键教训

### ✅ 应该做的

| 教训 | Twitter 经验 | WigoWago 应用 |
|------|-------------|---------------|
| **渐进式拆分** | 初期单体，按需拆分 | ✅ 当前单体，保留拆分接口 |
| **数据分区** | 后期分片，代价大 | ✅ 提前规划区域化 |
| **事件驱动** | 异步解耦 | ✅ Service Bus 同步 |
| **监控先行** | 后期补课 | ✅ Prometheus 内置 |

### ❌ 应该避免的

| 问题 | Twitter 代价 | WigoWago 预防 |
|------|-------------|---------------|
| **过度微服务化** | 100+ 服务，运维灾难 | ❌ 避免过早拆分 |
| **同步 RPC 依赖** | 级联故障 | ❌ 异步优先 |
| **忽略技术债** | 重写成本高 | ❌ 代码质量优先 |
| **忽略合规** | GDPR 罚款风险 | ✅ GDPR 内置设计 |

---

## 🏗️ 架构调整建议

### 当前设计评估

| 维度 | 当前设计 | 评估 | 建议 |
|------|---------|------|------|
| **区域化** | EU/NA 双集群 | ✅ 合理 | 保持 |
| **数据分区** | 区域分片 | ✅ 合理 | 保持 |
| **同步机制** | Global Sync Service | ⚠️ 可能过重 | 简化 |
| **微服务** | 单体 + 多数据源 | ✅ 合理 | 保持 |
| **事件驱动** | Service Bus | ✅ 合理 | 保持 |

### 调整方案

#### 方案 A: 简化 Global Sync (推荐)

**当前设计**:
```
EU Cluster → Global Sync Service ← NA Cluster
              (独立服务)
```

**调整后**:
```
EU Cluster ←→ Service Bus ←→ NA Cluster
     │                        │
     └──── Sync Workers ──────┘
     (内置于各区域 API)
```

**优势**:
- ✅ 减少一个独立服务
- ✅ 降低运维复杂度
- ✅ 同步逻辑内聚
- ✅ 未来可独立拆分

#### 方案 B: 保持当前设计

如果预计广告投放等需求会在 6 个月内上线，保持 Global Sync 独立服务是合理的。

---

## 📐 推荐的轻量架构

### 核心原则

1. **单体优先** - 业务逻辑在单体 API 内
2. **数据分区** - 区域化数据库
3. **异步通信** - Service Bus 跨区域同步
4. **无状态服务** - 便于水平扩展
5. **监控内置** - 从 Day 1 开始

### 架构图

```mermaid
graph TB
    subgraph Global[\"Global Layer (轻量)\"]
        AFD[Azure Front Door]
        DNS[Azure DNS]
    end

    subgraph EU[\"EU Cluster\"]
        EU_GW[API Gateway]
        EU_API[WigoWago API<br/>Stateless]
        EU_PG[(PostgreSQL<br/>EU Data)]
        EU_REDIS[(Redis<br/>Cache)]
        EU_SB[Service Bus<br/>Sync Queue]
    end

    subgraph NA[\"NA Cluster\"]
        NA_GW[API Gateway]
        NA_API[WigoWago API<br/>Stateless]
        NA_PG[(PostgreSQL<br/>NA Data)]
        NA_REDIS[(Redis<br/>Cache)]
        NA_SB[Service Bus<br/>Sync Queue]
    end

    AFD --> DNS
    DNS --> EU_GW
    DNS --> NA_GW

    EU_GW --> EU_API
    EU_API --> EU_PG
    EU_API --> EU_REDIS
    EU_API --> EU_SB

    NA_GW --> NA_API
    NA_API --> NA_PG
    NA_API --> NA_REDIS
    NA_API --> NA_SB

    EU_SB <-->|Async Sync| NA_SB

    style EU_API fill:#d4edda
    style NA_API fill:#d4edda
    style EU_SB fill:#fff3cd
    style NA_SB fill:#fff3cd
```

### 与之前设计的对比

| 组件 | 之前设计 | 调整后设计 | 变化 |
|------|---------|-----------|------|
| **Global Sync** | 独立服务 | 内置 Sync Workers | 简化 |
| **API 服务** | 区域单体 | 区域单体 | 保持 |
| **数据库** | 区域分片 | 区域分片 | 保持 |
| **事件总线** | Service Bus | Service Bus | 保持 |
| **广告投放** | Global Service | 未来扩展 | 延后 |

---

## 🚀 分阶段实施计划

### Phase 1: MVP (Week 1-8)

**目标**: 快速上线，验证业务

```
✅ 单体 API (Node.js + TypeScript)
✅ 区域数据库 (EU + NA)
✅ 基础同步 (Service Bus)
✅ Azure Front Door (全球负载均衡)
```

**不做**:
- ❌ Global Sync 独立服务
- ❌ 复杂微服务拆分
- ❌ 广告投放系统

### Phase 2: 增长 (Week 9-16)

**目标**: 支撑 10 万用户

```
✅ 性能优化 (缓存、索引)
✅ 监控完善 (Prometheus + Grafana)
✅ 自动化运维 (CI/CD)
✅ 同步优化 (延迟降低)
```

**评估**:
- 🔍 是否需要拆分 Global Sync
- 🔍 是否需要独立搜索服务
- 🔍 是否需要独立推荐系统

### Phase 3: 扩展 (Week 17-24)

**目标**: 支撑 100 万用户

根据 Phase 2 的评估结果决定：

**选项 A: 保持轻量** (如果用户 < 50 万)
- 继续优化单体
- 增加只读副本
- CDN 优化

**选项 B: 服务拆分** (如果用户 > 50 万)
- 拆分 Global Sync
- 独立搜索服务
- 独立推荐系统
- 广告投放平台

---

## 📊 技术选型对比

### 数据库

| 方案 | 优势 | 劣势 | 推荐 |
|------|------|------|------|
| **PostgreSQL (当前)** | 成熟、支持 JSON、扩展好 | 需要自己管理 | ✅ 保持 |
| **Supabase** | 托管、实时、Auth 内置 | 供应商锁定 | ⚠️ 备选 |
| **CockroachDB** | 全球分布式、强一致 | 复杂度高 | ❌ 过重 |

### 事件总线

| 方案 | 优势 | 劣势 | 推荐 |
|------|------|------|------|
| **Azure Service Bus (当前)** | 托管、可靠、跨区域 | Azure 绑定 | ✅ 保持 |
| **Kafka** | 开源、高吞吐 | 运维复杂 | ❌ 过重 |
| **Redis Streams** | 简单、低延迟 | 功能有限 | ⚠️ 备选 |

### 缓存

| 方案 | 优势 | 劣势 | 推荐 |
|------|------|------|------|
| **Redis (当前)** | 成熟、功能多 | 需要管理 | ✅ 保持 |
| **Azure Cache** | 托管、集成好 | Azure 绑定 | ✅ 推荐 |
| **Memcached** | 简单 | 功能少 | ❌ 不推荐 |

---

## 🎯 关键决策点

### 何时拆分 Global Sync?

**触发条件** (满足任一即拆分):
- [ ] 日活用户 > 10 万
- [ ] 同步延迟 > 5 秒 (持续)
- [ ] 需要独立扩展同步服务
- [ ] 广告投放需求明确

**拆分信号**:
- Sync Workers 占用 API 资源 > 20%
- 同步逻辑复杂度超过业务逻辑
- 需要独立的监控和告警

### 何时引入微服务？

**触发条件**:
- [ ] 团队规模 > 10 人
- [ ] 部署频率 < 每天 1 次
- [ ] 单服务代码 > 10 万行
- [ ] 故障隔离需求明确

**拆分优先级**:
1. 认证服务 (Auth)
2. 通知服务 (Notification)
3. 媒体处理 (Media Processing)
4. 搜索服务 (Search)
5. 推荐系统 (Recommendation)

---

## 📝 实施建议

### 立即行动

1. **保持当前单体架构**
   - 快速迭代
   - 降低复杂度
   - 专注业务

2. **简化 Global Sync**
   - 作为 API 内置模块
   - 使用 Service Bus 异步
   - 预留独立接口

3. **投资监控和日志**
   - Prometheus + Grafana
   - 结构化日志
   - 分布式追踪 (OpenTelemetry)

4. **自动化运维**
   - CI/CD 流水线
   - 基础设施即代码 (Terraform)
   - 自动扩缩容

### 未来 6 个月观察指标

| 指标 | 目标 | 预警 |
|------|------|------|
| **日活用户** | < 10 万 | > 5 万开始规划拆分 |
| **API 延迟 (P95)** | < 200ms | > 150ms 优化 |
| **同步延迟** | < 1 秒 | > 500ms 关注 |
| **错误率** | < 0.1% | > 0.05% 告警 |
| **部署频率** | > 每天 1 次 | < 每周 1 次 改进 |

---

## 📚 参考资源

### Twitter 架构文章

- [Twitter Engineering Blog](https://blog.twitter.com/engineering)
- [How X Handles Millions of Tweets](https://blog.stackademic.com/how-x-formerly-twitter-handles-millions-of-tweets-every-second-22d4fadb8d79)
- [Rebuilding Twitter's Public API](https://blog.x.com/engineering/en_us/topics/infrastructure/2020/rebuild_twitter_public_api_2020)

### 架构最佳实践

- [Martin Fowler - Microservices](https://martinfowler.com/microservices/)
- [AWS Well-Architected Framework](https://aws.amazon.com/architecture/well-architected/)
- [Google SRE Book](https://sre.google/books/)

---

*本文档基于 Twitter/X 架构演进经验，结合 WigoWago 实际需求制定。建议每 3 个月回顾一次，根据业务发展调整。*
