# WigoWago V2 - Global Feed 结构设计

> **版本**: 1.0  
> **创建日期**: 2026-04-08  
> **参考架构**: Twitter/Facebook/Instagram Feed 系统

---

## 📊 Feed 生成策略

基于业界成熟设计，采用 **混合模式 (Hybrid Push-Pull)**：

| 策略 | 适用场景 | 优点 | 缺点 |
|------|----------|------|------|
| **Push (Fan-out)** | 普通用户 (<1000 粉丝) | 读取快、延迟低 | 写入慢、存储冗余 |
| **Pull (Fan-in)** | 名人用户 (≥1000 粉丝) | 写入快、存储少 | 读取慢、需聚合 |
| **Hybrid** | 混合模式 | 平衡读写性能 | 实现复杂 |

---

## 🗄️ 数据模型设计

### 1. 预计算 Feed 表 (Push 模式)

```sql
-- 用户 Feed 表 (每个用户一条记录，存储 Post ID 列表)
CREATE TABLE user_feed (
    user_id BIGINT NOT NULL,
    region VARCHAR(16) NOT NULL,  -- 'EU' or 'NA'
    post_ids BIGINT[] NOT NULL,   -- 预计算的 Post ID 列表 (最新 500 条)
    last_updated TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, region)
);

-- 分区表 (按区域)
CREATE TABLE user_feed_eu PARTITION OF user_feed FOR VALUES IN ('EU');
CREATE TABLE user_feed_na PARTITION OF user_feed FOR VALUES IN ('NA');

-- 索引
CREATE INDEX idx_user_feed_last_updated ON user_feed (last_updated);
```

### 2. Global Post Index 表

```sql
-- Global Post 索引 (用于跨区域查询)
CREATE TABLE global_post_index (
    post_id BIGINT PRIMARY KEY,
    author_id BIGINT NOT NULL,
    author_region VARCHAR(16) NOT NULL,
    content_preview TEXT,
    visibility VARCHAR(20) NOT NULL,  -- 'GLOBAL', 'REGIONAL', 'PRIVATE'
    post_type VARCHAR(20) NOT NULL,   -- 'TEXT', 'IMAGE', 'VIDEO'
    hashtags TEXT[],                   -- 用于搜索
    mentions BIGINT[],                 -- 提及的用户 ID
    stats JSONB NOT NULL DEFAULT '{"likes":0,"comments":0,"shares":0}',
    created_at TIMESTAMPTZ NOT NULL,
    synced_at TIMESTAMPTZ NOT NULL,
    gdpr_compliant BOOLEAN NOT NULL DEFAULT true,
    user_consent BOOLEAN NOT NULL DEFAULT false
);

-- 索引优化
CREATE INDEX idx_global_post_author ON global_post_index (author_id);
CREATE INDEX idx_global_post_visibility ON global_post_index (visibility);
CREATE INDEX idx_global_post_created ON global_post_index (created_at DESC);
CREATE INDEX idx_global_post_hashtags ON global_post_index USING GIN (hashtags);
CREATE INDEX idx_global_post_gdpr ON global_post_index (gdpr_compliant, user_consent);

-- 分区表 (按可见性)
CREATE TABLE global_post_public PARTITION OF global_post_index 
    FOR VALUES IN ('GLOBAL', 'REGIONAL');
CREATE TABLE global_post_private PARTITION OF global_post_index 
    FOR VALUES IN ('PRIVATE');
```

### 3. Redis Cache 结构

```redis
# 用户 Feed Cache (预计算，5 分钟过期)
user:feed:{user_id}:{region} → [post_id_1, post_id_2, ..., post_id_n]
TTL: 300s

# Post 详情 Cache (长期缓存)
post:detail:{post_id} → {JSON post data}
TTL: 86400s (24h)

# 用户关注列表 Cache (长期缓存)
user:following:{user_id} → [followed_user_id_1, followed_user_id_2, ...]
TTL: 3600s (1h)

# 热点 Post 统计 (实时更新)
post:stats:{post_id}:likes → {count}
post:stats:{post_id}:comments → {count}
post:stats:{post_id}:shares → {count}
TTL: 86400s (24h)
```

---

## 🔄 Feed 生成流程

### Push 模式 (普通用户 <1000 粉丝)

```
用户创建 Post
    ↓
API 写入 Regional DB
    ↓
API → Global Sync (HTTP POST /sync)
    ↓
Global Sync 验证 GDPR 合规
    ↓
Global Sync 查询粉丝列表
    ↓
遍历每个粉丝 (上限 1000)
    ↓
更新粉丝 Feed Cache (Redis)
    ↓
插入 Post ID 到列表头部
```

### Pull 模式 (名人用户 ≥1000 粉丝)

```
用户请求 Feed
    ↓
查询 Feed Cache (Redis)
    ↓
Cache Miss → 触发 Feed 生成
    ↓
查询关注列表
    ↓
聚合关注的用户 Posts (Global Index DB)
    ↓
排序 (时间/算法)
    ↓
截取最新 N 条
    ↓
缓存结果 (Redis)
    ↓
返回 Feed
```

---

## 📡 Feed API 设计

### 获取 Feed

```typescript
// GET /api/v1/feed
interface GetFeedRequest {
  userId: string;
  limit?: number;        // 默认 20，最大 100
  offset?: string;       // 分页游标 (post_id)
  feedType?: 'following' | 'global' | 'trending';
}

interface GetFeedResponse {
  posts: FeedPost[];
  nextCursor?: string;
  hasMore: boolean;
}

interface FeedPost {
  postId: string;
  authorId: string;
  authorName: string;
  authorAvatar?: string;
  content: string;
  media?: MediaAttachment[];
  visibility: 'GLOBAL' | 'REGIONAL' | 'FOLLOWERS' | 'PRIVATE';
  stats: {
    likes: number;
    comments: number;
    shares: number;
  };
  createdAt: number;
  isLiked?: boolean;     // 当前用户是否点赞
  isFollowed?: boolean;  // 当前用户是否关注作者
}
```

### Feed 排序算法

```typescript
// Feed 排序策略
enum FeedRankingStrategy {
  CHRONOLOGICAL = 'chronological',     // 时间顺序
  RELEVANCE = 'relevance',             // 相关性算法
  TRENDING = 'trending',               // 热门内容
}

// 相关性评分公式 (简化版)
function calculateRelevanceScore(post: Post, user: User): number {
  const recencyScore = calculateRecencyScore(post.createdAt);    // 时间衰减
  const engagementScore = calculateEngagementScore(post.stats);  // 互动率
  const affinityScore = calculateAffinityScore(post.authorId, user.id); // 亲密度
  const contentTypeScore = calculateContentTypeScore(post.type, user.preferences); // 内容偏好
  
  return (
    recencyScore * 0.3 +
    engagementScore * 0.3 +
    affinityScore * 0.3 +
    contentTypeScore * 0.1
  );
}
```

---

## 🏗️ 缓存架构

### 多级缓存

```
L1 Cache (本地内存) → TTL: 1 分钟
    ↓
L2 Cache (Redis) → TTL: 5 分钟 - 24 小时
    ↓
L3 Cache (Database) → PostgreSQL
```

### 缓存预热

```typescript
// 定时任务：预热活跃用户 Feed
async function warmupActiveUserFeeds(): Promise<void> {
  const activeUsers = await getActiveUsers(lastActiveMinutes: 60);
  
  for (const user of activeUsers) {
    const feed = await generateFeed(user.id, limit: 50);
    await redis.set(`user:feed:${user.id}:eu`, JSON.stringify(feed), 'EX', 300);
  }
}

// 事件驱动：关注新用户时预热
async function onFollowUser(followerId: string, followedId: string): Promise<void> {
  // 清除 follower 的旧 Feed Cache
  await redis.del(`user:feed:${followerId}:eu`);
  
  // 触发异步 Feed 生成
  await queue.push('generate-feed', { userId: followerId });
}
```

---

## ⚡ 性能目标

| 指标 | 目标 | 测量方式 |
|------|------|----------|
| **Feed 加载延迟** | P99 < 200ms | API Gateway 监控 |
| **数据库查询** | 单次 < 50ms | Application Insights |
| **缓存命中率** | > 95% | Redis 监控 |
| **跨区域同步** | 延迟 < 5 秒 | Global Sync 监控 |

---

## 📈 扩展路径

| 阶段 | 用户规模 | Feed 策略 | 技术选型 |
|------|----------|----------|----------|
| **MVP** | < 10 万 | Hybrid Push-Pull | PostgreSQL + Redis |
| **Growth** | < 100 万 | 引入 Elasticsearch | ES for 搜索/Feed |
| **Scale** | > 100 万 | 流式处理 | Kafka + Flink |
