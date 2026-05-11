# Global Sync Service — 测试覆盖率提升计划

> **Goal:** 将整体覆盖率从 ~35% 提升至 70%+，核心业务逻辑（service + handler + consumer）从 ~20% 提升至 60%+

**当前覆盖:** service 26% | handler 19% | consumer 2% | sync 45%
**目标覆盖:** service 75%+ | handler 60%+ | consumer 50%+ | sync 70%+

---

## 测试策略

所有测试遵循 **mock 外部依赖** 原则：
- **DB**: 使用 `pgxmock`（pgx 官方 mock）模拟 SQL 查询
- **Redis**: 使用 `miniredis` 内嵌 Redis
- **HTTP**: 使用 `httptest` 模拟请求
- 不连接真实数据库/Redis/外部服务

---

### Phase 1: Service 层 — GlobalIndexService（26% → 75%）

核心数据访问层，最重要。

#### Task 1.1: InsertPost 单元测试
- 测试正常插入（新 post_slug，无冲突）
- 测试 upsert（ON CONFLICT DO UPDATE）
- 测试带 hashtags/mentions/mediaUrls
- 测试 nil AuthorProfile（authorNickname/authorAvatarURL 为 NULL）
- 文件: `internal/service/global_index_test.go`

#### Task 1.2: UpdatePost / DeletePost 单元测试
- 测试正常更新（rows affected > 0）
- 测试更新不存在的 post（rows=0，fallback 到 InsertPost）
- 测试删除成功
- 测试删除不存在的 post（rows=0，静默）

#### Task 1.3: UpdateStats 单元测试
- 测试更新 likes/comments/shares/views

#### Task 1.4: GetPost / GetPostByUid 单元测试
- 测试查询存在/不存在的 post
- 测试 mediaUrls 解析（逗号分隔 → []string）
- 测试 authorNickname/authorAvatarURL 为 NULL

#### Task 1.5: GetPostsByAuthor / GetPostsFromAuthors / GetGlobalPosts / GetTrendingPosts
- 测试多条结果
- 测试空结果
- 测试 COALESCE post_slug 正确工作

#### Task 1.6: User Index 操作
- UpsertUserIndex（测试 INSERT + ON CONFLICT UPDATE）
- FindRegionByEmailHash（存在/不存在）
- FindRegionByUID（存在/不存在）
- GetAllUserIndexEntries

#### Task 1.7: 辅助函数
- extractHashtags（各种 content 模式）
- truncatePreview（截断/不截断）
- pgtypeArray.Scan（NULL, 空数组, 多元素）

---

### Phase 2: Service 层 — FeedGenerator（0% → 65%）

#### Task 2.1: HandleNewPost — 决策逻辑
- 测试 followerCount < pushThreshold → 走 pushMode
- 测试 followerCount >= pushThreshold → pull 模式（日志记录）
- 测试 getFollowerCount 报错 → fallback 到 pull

#### Task 2.2: HandleDeletedPost
- 测试日志记录的删除事件

#### Task 2.3: GetFeed — 路由逻辑
- 测试 feedType="following" → getFollowingFeed
- 测试 feedType="global" → getGlobalFeed
- 测试 feedType="trending" → getTrendingFeed
- 测试未知 feedType → default to following

#### Task 2.4: pushMode — fan-out 逻辑
- 测试有 followers → 逐个写入 Redis ZSET
- 测试无 followers → 提前返回
- 测试部分 Redis 写入失败 → 继续处理剩余

#### Task 2.5: pullMode — getFollowingFeed / getGlobalFeed / getTrendingFeed
- 测试 Redis 缓存命中 → 直接返回
- 测试缓存未命中 → 查询 GlobalIndex → 缓存 → 返回
- 测试空结果

#### Task 2.6: CalculateScore（已有部分覆盖率）
- 补充边界值测试（views=0, 极旧 post, 未来时间）

---

### Phase 3: Service 层 — EventLog + GDPRChecker（0% → 60%）

#### Task 3.1: SyncEventLogService — IsProcessed / MarkProcessed
- 测试 idempotency 检查（已处理/未处理）
- 测试标记已处理

#### Task 3.2: GDPRChecker — Check 决策
- 测试 TIER_1 (PII) + no consent → 拒绝
- 测试 TIER_2 (UGC) + consent → 允许
- 测试 crossBorderOk false → 拒绝
- 测试各种 DataCategory 组合

---

### Phase 4: Handler 层（19% → 60%）

#### Task 4.1: SyncHandler — HandleSync / HandleCrossSync
- 测试有效 POST → 202 Accepted
- 测试无效 JSON → 400
- 测试缺少 eventId/eventType → 400
- 测试 processEvent 报错 → 500

#### Task 4.2: SyncHandler — HandleGetPost / HandleGetPostByUid
- 测试查询存在 post → 200 + JSON body
- 测试查询不存在 → 404
- 测试无效 uid → 400

#### Task 4.3: SyncHandler — Tag endpoints
- HandleSearchTags（keyword + limit）
- HandlePopularTags（limit）
- HandleGetTag（存在/不存在）
- HandleGetTagRegions

#### Task 4.4: SyncHandler — routeEvent 分支
- 测试 POST_CREATED → InsertPost + HandleNewPost
- 测试 POST_UPDATED → UpdatePost
- 测试 POST_DELETED → DeletePost + HandleDeletedPost
- 测试 POST_STATS_UPDATED → handleStatsUpdated
- 测试 TAG_CREATED/TAG_UPDATED → UpsertTag
- 测试 TAG_DELETED → DeleteTag
- 测试 TAG_STATS_UPDATED → UpdateStats
- 测试未知 eventType → 静默返回

#### Task 4.5: UserIndexHandler 全部端点
- HandleCheckUser（存在/不存在 email_hash）
- HandleUpsertUser（有效请求/缺少参数）
- HandleGetUserRegion（存在/不存在 uid）
- HandleGetAllUsers（空/有数据）

---

### Phase 5: Consumer 层（2% → 50%）

#### Task 5.1: routeEvent 分支（mock 全部依赖）
- 与 handler 的 routeEvent 相同的 10 个分支测试
- 测试 stats update 流程（mock regionalDB）

#### Task 5.2: HandleMessage 完整流程
- 测试正常处理（解析 → idempotency → GDPR → route → mark）
- 测试 idempotency 跳过
- 测试 GDPR 拒绝
- 测试解析失败 → retry

---

### Phase 6: Sync 层增强（45% → 70%）

#### Task 6.1: CrossSyncService — Broadcast 补充
- 测试多 peer 广播（部分成功部分失败）
- 测试 idempotency（重复 eventID 跳过）

#### Task 6.2: UserIndexReconciler
- 测试周期性拉取 peer 数据
- 测试 diff merge 逻辑

---

### Phase 7: 验证 & 收尾

#### Task 7.1: 全量覆盖率报告
- 运行 `go test ./... -coverprofile=coverage.out`
- 输出逐包覆盖率

#### Task 7.2: 清理
- 确保所有测试通过 `-race` 标志
- 提交代码
