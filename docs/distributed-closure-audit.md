# WigoWago 分布式功能闭环审计报告

## 审计范围
- **API** (pv-wigowago-api): 事件发布 + Global Index 消费
- **GlobalSync** (pv-global-sync-service): 事件处理 + 索引服务 + Feed 生成
- **App**: Feed 消费 + 跨区数据展示

---

## 1. 事件链路: API → GlobalSync

### 1.1 事件类型全覆盖

| 事件类型 | API 发布 | HTTP Handler 处理 | MQ Consumer 处理 | 闭环? |
|----------|----------|-------------------|-----------------|--------|
| POST_CREATED | ✅ post.service.ts:393 | ✅ InsertPost + Feed push | ✅ InsertPost + Feed push | ✅ |
| POST_UPDATED | ✅ post.service.ts:707,1977 | ✅ UpdatePost | ✅ UpdatePost | ✅ |
| POST_DELETED | ✅ post.service.ts:730,751 | ✅ DeletePost + Feed invalidate | ✅ DeletePost | ✅ |
| POST_STATS_UPDATED | ✅ post-interaction.service.ts:225 | ✅ UpdateStats | ✅ UpdateStats | ✅ |
| TAG_CREATED | ✅ tag.service.ts:99 | ✅ UpsertTag | ❌ 落 default（丢弃） | ⚠️ |
| TAG_UPDATED | ✅ tag.service.ts:185 | ✅ UpsertTag | ❌ 落 default（丢弃） | ⚠️ |
| TAG_DELETED | ✅ tag.service.ts:218 | ✅ DeleteTag | ❌ 落 default（丢弃） | ⚠️ |
| TAG_STATS_UPDATED | ❌ 定义但从未发送 | ✅ UpdateStats | ❌ 落 default（丢弃） | 🔴 |

### 1.2 HTTP vs RocketMQ 双路径分析

**HTTP 路径**（API → POST `/sync/content`）:
- `SyncHandler.HandleSync()` → `processEvent()` → `handler.routeEvent()`
- `handler.routeEvent()` 覆盖全部 8 种事件类型 ✅
- 当前 API 使用此路径发送所有事件

**RocketMQ 路径**（MQ → Consumer）:
- `SyncConsumer.HandleMessage()` → `consumer.routeEvent()`
- `consumer.routeEvent()` 仅覆盖 4 种事件（POST_CREATED/UPDATED/DELETED/STATS_UPDATED）
- TAG 事件全部落入 `default` 分支 → `log.Warn` + 静默丢弃 🔴

```go
// sync_consumer.go:170-198
func (c *SyncConsumer) routeEvent(ctx context.Context, event *model.CrossRegionSyncEvent) error {
    switch event.EventType {
    case model.EventTypePostCreated:    // ✅
    case model.EventTypePostUpdated:    // ✅
    case model.EventTypePostDeleted:    // ✅
    case model.EventTypePostStatsUpdated: // ✅
    default:
        c.log.Warn("Unknown event type, skipping")  // 🔴 TAG 事件被丢弃
        return nil
    }
}
```

### 1.3 跨区广播

- `processEvent()` 中: 本地 HTTP 事件 → fire-and-forget 广播到 peer `/sync/cross-sync`
- Peer 收到后走相同的 `processEvent()` → `handler.routeEvent()` ✅
- 跨区广播覆盖全部 8 种事件 ✅

---

## 2. 数据消费链路: GlobalSync → API

### 2.1 Global Index 查询端点

| 端点 | 用途 | API 消费处 | 闭环? |
|------|------|-----------|--------|
| POST /index/users/check | 注册时检查 email 全局唯一 | auth.service.ts | ✅ |
| POST /index/users/upsert | 注册后同步用户索引 | auth.service.ts | ✅ |
| GET /index/posts/{uid} | 获取跨区帖子详情 | global-sync-client.ts | ✅ |
| GET /index/tags/search | 全局标签搜索 | tag.service.ts | ✅ |
| GET /index/tags/popular | 热门标签 | tag.service.ts | ✅ |
| GET /index/tags/{tagUid} | 获取单个标签 | global-sync-client.ts | ✅ |
| GET /index/tags/{tagUid}/regions | 确定 tag 所在区域 | tag.service.ts (代理路由) | ✅ |
| GET /feed/{userId} | 混合 Feed | post.service.ts (listPostsWithGlobalFeed) | ✅ |
| GET /index/user/region | 用户区域查询 | user.controller.ts (代理路由) | ✅ |

### 2.2 Feed 类型覆盖

| Feed 类型 | 数据源 | 实现状态 |
|-----------|--------|----------|
| following | 从 authorIDs 拉取 global_post_index | ✅ GetPostsFromAuthors |
| global | 全部公开帖（最近） | ✅ GetGlobalPosts |
| trending | 24h 内高互动帖 | ✅ GetTrendingPosts |

---

## 3. GlobalSync 内部完整性

### 3.1 Schema 与命名

| 项 | 状态 | 备注 |
|----|------|------|
| global_post_index PK = uid | ✅ | migration 016 完成 |
| global_tag_index | ✅ | migration 014 创建 |
| users_global_index PK = uid | ✅ | migration 011 重建 |
| author_uid FK | ✅ | 索引 idx_gpi_author_uid |
| post_slug 残余 | ✅ 已清理 | 全部替换为 uid |

### 3.2 数据库分布

| 组件 | SEA DB | EU DB | 说明 |
|------|--------|-------|------|
| Regional DB | wigowago_dev | wigowago-eu | 各自独立 |
| Global Index DB | wigowago_global_index (SEA PG) | wigowago_global_index (EU PG) | **各自独立！** |
| User Reconciler | SEA→EU / EU→SEA | 每 5 分钟同步 | ✅ |

### 3.3 Migration 状态

| 集群 | 版本 | 状态 |
|------|------|------|
| SEA | v2.17 (migration 016 applied) | ✅ Running |
| EU | v2.17 (migration 016 applied) | ✅ Running |

---

## 4. 断点清单

### 🔴 CRITICAL: TAG_STATS_UPDATED 从未发布

- **位置**: `pv-wigowago-api/src/domain/tag/tag.service.ts`
- **影响**: tag usage count 变更后 Global Index 中的 `postCount` 永不同步
- **触发操作**: `incrementUsageCount()` (line 327), `decrementUsageCount()` (line 334), `associateTagsWithPost()` (line 522)
- **修复**: 在 `incrementUsageCount` / `decrementUsageCount` 中调用 `globalSyncService.send({ eventType: 'TAG_STATS_UPDATED', ... })`

### 🔴 CRITICAL: Consumer 不处理 TAG 事件

- **位置**: `pv-global-sync-service/internal/consumer/sync_consumer.go:170`
- **影响**: 如有任何事件通过 RocketMQ 到达（而非 HTTP），4 种 TAG 事件全被丢弃
- **修复**: 在 `consumer.routeEvent()` 中添加 TAG 事件 case（与 handler 保持一致）

### 🟡 MEDIUM: Global Index 各自独立（非共享）

- **当前**: SEA 和 EU 各有独立的 `wigowago_global_index` DB
- **影响**: 两套 Global Index 独立维护，通过 User Reconciler 同步 User Index，但 Post Index 和 Tag Index 靠跨区广播同步
- **风险**: 如果跨区广播失败，两套索引会不一致

### 🟡 MEDIUM: Author Metadata 不完整

- `HandleNewPost` 和 feed 查询返回的 `authorNickname` / `authorAvatarURL` 在 `InsertPost` 时插入，但 `UpdatePost` 不更新这两个字段
- **影响**: 用户改名/换头像后，已缓存的 Global Index 帖子的作者元数据过期

### 🟢 LOW: GetPost 和 GetPostByUid 是重复代码

- 两者完全等价，都查询 `WHERE uid = $1`
- 可简化为一个方法

---

## 5. 全链路测试矩阵

| 场景 | API → Sync | Cross-Region | Feed | 状态 |
|------|-----------|--------------|------|------|
| 创建帖子 (SEA) → EU 可见 | ✅ | ✅ | ✅ | 已验证 |
| 删除帖子 → Global Index 清除 | ✅ | ✅ | - | 已验证 |
| 互动统计 → Global Index 更新 | ✅ | - | - | 已验证 |
| 创建 Tag → Global Tag Index | ✅ HTTP | ⚠️ MQ 路径断 | - | 部分 |
| Tag 使用计数 → Tag Index 更新 | ❌ | ❌ | - | 断点 |
| Feed (following) | ✅ | ✅ | ✅ | 已验证 |
| Feed (global) | ✅ | ✅ | ✅ | 已验证 |
| Feed (trending) | ✅ | ✅ | ✅ | 已验证 |
| 用户注册 → User Index | ✅ | ✅ | ✅ | 已验证 |

---

## 6. 优先级修复建议

1. **[P0]** `consumer.routeEvent()` 添加 TAG 事件处理（对齐 handler）
2. **[P0]** `tag.service.ts` 发布 `TAG_STATS_UPDATED` 事件
3. **[P1]** `UpdatePost` 也更新 `author_nickname` / `author_avatar_url`
4. **[P2]** 合并 `GetPost` / `GetPostByUid` 重复代码
