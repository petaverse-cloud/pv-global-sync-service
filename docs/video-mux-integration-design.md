# WigoWago Video Streaming — Mux 集成设计文档 v1

> **版本**: V1.0 Draft
> **日期**: 2026-04-17
> **状态**: 待评审
> **目标**: 为 WigoWago API 引入视频上传、转码、HLS 自适应播放能力

---

## 1. 架构总览

### 1.1 当前 vs 目标

```
当前 (raw upload):
App → WigoWago API → Azure Blob Storage → CDN URL
问题: 无自适应码率、存储浪费、播放体验差

目标 (Mux):
App → WigoWago API → 获取 Mux Signed Upload URL
App → Mux Direct Upload (直传，不经过我们的服务器)
Mux → 自动转码 HLS (240p/360p/480p/720p/1080p)
Mux → webhook → WigoWago API → 更新 Post 状态
用户播放 → Mux Multi-CDN (Fastly + Akamai)
```

### 1.2 数据流

```
┌──────────┐    1.请求上传     ┌─────────────────┐     ┌──────────┐
│  Flutter  │ ─────────────────▶ │ WigoWago API     │────▶│  Mux API │
│    App    │ ◀──────────────── │ POST /videos     │◀────│ (SaaS)  │
└──────────┘    返回 Signed URL └─────────────────┘     └──────────┘
      │                                                    ▲
      │ 2.直传视频 (不经过 WigoWago)                       │
      ▼                                                    │
┌──────────┐     3.自动转码 HLS                           │
│   Mux    │ ──────────────────────────────────────────────┤
│ Platform │     4.转码完成 webhook                        │
└──────────┘                                              │
      │ 5.播放 (Multi-CDN)                                │
      ▼                                                    │
┌──────────┐                                              │
│  Flutter  │◀─────────────────────────────────────────────┘
│    App    │     6.playback URL (HLS .m3u8)
└──────────┘
```

---

## 2. API 设计

### 2.1 新增端点

#### POST /videos/upload-init

初始化视频上传，获取 Mux Direct Upload URL。

**请求**:
```json
{
  "postId": 12345,
  "filename": "my_dog_run.mp4",
  "duration": 30,
  "resolution": "1080p"
}
```

**响应 (200)**:
```json
{
  "uploadUrl": "https://storage.mux.com/uploads/abc123...",
  "videoId": "vid_mux_xxxxx",
  "status": "waiting_for_upload",
  "expiresAt": "2026-04-18T12:00:00Z"
}
```

#### POST /videos/upload-complete

通知服务器视频已上传完毕（可选，App 也可直接依赖 webhook）。

**请求**:
```json
{
  "videoId": "vid_mux_xxxxx",
  "postId": 12345
}
```

**响应 (200)**:
```json
{
  "status": "processing"
}
```

#### POST /webhooks/mux

Mux webhook 回调端点（不需要认证，通过签名验证）。

**请求 (Mux 发送)**:
```json
{
  "type": "video.asset.ready",
  "data": {
    "id": "asset_mux_xxxxx",
    "upload_id": "upload_mux_yyyyy",
    "status": "ready",
    "playback_ids": [
      { "id": "playback_zzz", "policy": "public" }
    ],
    "max_stored_resolution": "1080p",
    "max_stored_frame_rate": 30,
    "duration": 30.5,
    "aspect_ratio": "16:9",
    "static_renditions": {
      "status": "ready",
      "files": [
        { "name": "thumbnail.jpg", "height": 720, "width": 1280 }
      ]
    }
  }
}
```

### 2.2 修改现有端点

#### POST /posts — 响应体新增 video 字段

```json
{
  "postId": 12345,
  "content": "My dog running!",
  "authorRegion": "sea",
  "mediaUrls": [
    "https://image.cdn.wigowago.com/photo1.jpg"
  ],
  "video": {
    "assetId": "asset_mux_xxxxx",
    "playbackId": "playback_zzz",
    "playbackUrl": "https://stream.mux.com/playback_zzz.m3u8",
    "thumbnailUrl": "https://image.mux.com/playback_zzz/thumbnail.jpg",
    "duration": 30.5,
    "maxResolution": "1080p",
    "status": "ready"
  },
  "createdAt": "2026-04-17T12:00:00Z"
}
```

`video.status` 可能值: `waiting` | `processing` | `ready` | `errored`

---

## 3. Mux 集成详细设计

### 3.1 Direct Upload 流程

```typescript
// wigowago-api/src/domain/video/video.service.ts

import Mux from '@mux/mux-node';

const mux = new Mux({
  tokenId: process.env.MUX_TOKEN_ID,
  tokenSecret: process.env.MUX_TOKEN_SECRET,
});

async function createUploadUrl(postId: number, filename: string) {
  // 1. 创建 Mux Upload
  const upload = await mux.video.uploads.create({
    new_asset_settings: {
      playback_policy: ['public'],
      encoding_tier: 'baseline',  // 或 'smart'（per-title 编码，更贵但更省带宽）
      mp4_support: 'none',        // 不需要 MP4 下载，只要 HLS
      normalize_audio: true,
    },
    cors_origin: '*',  // 生产环境应限制为 wigowago.com
  });

  // 2. 关联 upload 到 post (临时存储)
  await this.db.query(
    `UPDATE posts SET mux_upload_id = $1, video_status = 'waiting' WHERE post_id = $2`,
    [upload.id, postId]
  );

  return {
    uploadUrl: upload.url,
    uploadId: upload.id,
    expiresAt: new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString(),
  };
}
```

### 3.2 Webhook 处理

```typescript
// wigowago-api/src/infra/webhook/mux-webhook.handler.ts

async function handleMuxWebhook(event: MuxWebhookEvent) {
  if (event.type !== 'video.asset.ready') {
    return { status: 'ignored' };
  }

  const asset = event.data;
  const playbackId = asset.playback_ids?.[0]?.id;
  if (!playbackId) return { status: 'error', message: 'no playback_id' };

  // 1. 查找关联的 post (通过 upload_id)
  const post = await this.db.query(
    `SELECT post_id FROM posts WHERE mux_upload_id = $1`,
    [asset.upload_id]
  );

  if (!post) {
    this.log.warn('Post not found for mux upload', { uploadId: asset.upload_id });
    return { status: 'not_found' };
  }

  // 2. 更新 post
  const playbackUrl = `https://stream.mux.com/${playbackId}.m3u8`;
  const thumbnailUrl = `https://image.mux.com/${playbackId}/thumbnail.jpg`;

  await this.db.query(
    `UPDATE posts SET
      mux_asset_id = $1,
      mux_playback_id = $2,
      video_playback_url = $3,
      video_thumbnail_url = $4,
      video_duration = $5,
      video_max_resolution = $6,
      video_status = 'ready',
      updated_at = NOW()
    WHERE post_id = $7`,
    [asset.id, playbackId, playbackUrl, thumbnailUrl,
     asset.duration, asset.max_stored_resolution, post.post_id]
  );

  // 3. 通知 Global Sync Service (新 post 的 video 已就绪)
  await this.globalSyncClient.notifyVideoReady(post.post_id, playbackUrl);

  return { status: 'ok' };
}
```

### 3.3 错误处理

```typescript
async function handleMuxError(event: MuxWebhookEvent) {
  if (event.type !== 'video.asset.errored') return;

  const post = await this.db.query(
    `SELECT post_id FROM posts WHERE mux_upload_id = $1`,
    [event.data.upload_id]
  );

  if (post) {
    await this.db.query(
      `UPDATE posts SET video_status = 'errored', video_error = $1
       WHERE post_id = $2`,
      [event.data.errors?.[0]?.messages?.join('; '), post.post_id]
    );

    // 通知 Global Sync 此 post 有 video 错误
    await this.globalSyncClient.notifyVideoError(post.post_id);
  }
}
```

---

## 4. 数据库变更

### 4.1 Regional DB `posts` 表新增列

```sql
-- Migration: 0XX_add_video_columns.sql
ALTER TABLE posts
  ADD COLUMN IF NOT EXISTS mux_upload_id VARCHAR(64),
  ADD COLUMN IF NOT EXISTS mux_asset_id VARCHAR(64),
  ADD COLUMN IF NOT EXISTS mux_playback_id VARCHAR(64),
  ADD COLUMN IF NOT EXISTS video_playback_url TEXT,
  ADD COLUMN IF NOT EXISTS video_thumbnail_url TEXT,
  ADD COLUMN IF NOT EXISTS video_duration DECIMAL(8,2),
  ADD COLUMN IF NOT EXISTS video_max_resolution VARCHAR(16),
  ADD COLUMN IF NOT EXISTS video_status VARCHAR(16) DEFAULT 'none',
  ADD COLUMN IF NOT EXISTS video_error TEXT;

CREATE INDEX IF NOT EXISTS idx_posts_mux_upload ON posts(mux_upload_id);
CREATE INDEX IF NOT EXISTS idx_posts_video_status ON posts(video_status)
  WHERE video_status IS NOT NULL AND video_status != 'none';
```

### 4.2 Global Index 无变更

`global_post_index.media_urls TEXT[]` 已足够存储 Mux playback URL。
视频 URL 作为 `mediaUrls` 数组中的一个元素，与图片 URL 同等处理。

如需要更细粒度控制，可新增：

```sql
ALTER TABLE global_post_index
  ADD COLUMN IF NOT EXISTS video_playback_url TEXT,
  ADD COLUMN IF NOT EXISTS video_thumbnail_url TEXT,
  ADD COLUMN IF NOT EXISTS video_duration DECIMAL(8,2);
```

---

## 5. Global Sync Service 变更

### 5.1 无需变更

现有 `EventPayload.mediaUrls` 是 `[]string` 类型，可以自然包含 Mux 的 playback URL。

```go
// 当前代码已支持
event.Payload.MediaURLs = []string{
    "https://image.cdn.wigowago.com/photo.jpg",
    "https://stream.mux.com/playback_xxx.m3u8",  // 新增视频 URL
}
```

### 5.2 可选增强 (Phase 2)

如果需要单独同步视频元信息（duration、resolution）：

```go
type EventPayload struct {
    // ...existing fields...
    MediaURLs    []string   `json:"mediaUrls,omitempty"`
    VideoURL     string     `json:"videoUrl,omitempty"`       // 新增
    VideoThumb   string     `json:"videoThumbnail,omitempty"` // 新增
    VideoDuration float64   `json:"videoDuration,omitempty"`  // 新增
}
```

**建议**: Phase 1 不需要。视频信息通过 `mediaUrls` 传递即可。

---

## 6. GDPR 合规

### 6.1 数据分类

| 数据类型 | GDPR 分类 | 说明 |
|---------|-----------|------|
| 视频内容 | TIER_2 (UGC) | 用户上传的宠物视频 |
| 视频缩略图 | TIER_4 (Media) | 自动生成的缩略图 |
| 播放统计 | TIER_3 (System) | 观看次数、缓冲率 |

### 6.2 Mux 数据驻留

Mux 默认将数据存储在美国。对于 EU 用户：
- 需要用户明确同意跨边境传输 (`userConsent: true`)
- 现有 GDPR Checker 已处理此逻辑，**无需改动**
- 审计日志自动记录（`cross_border_audit_log` 已有）

### 6.3 数据删除

用户删除账号时：
1. 删除 Regional DB 中的 posts
2. 调用 Mux API 删除对应的 assets
3. Global Sync 传播 `POST_DELETED` 事件（已有机制）

```typescript
async function deleteUserVideos(userId: number) {
  const posts = await this.db.query(
    `SELECT mux_asset_id FROM posts WHERE author_id = $1 AND mux_asset_id IS NOT NULL`,
    [userId]
  );

  for (const post of posts) {
    await mux.video.assets.delete(post.mux_asset_id);
  }
}
```

---

## 7. K8s 配置

### 7.1 Secret

```yaml
# wigowago-api secret
apiVersion: v1
kind: Secret
metadata:
  name: wigowago-api-secret
  namespace: wigowago-dev
type: Opaque
stringData:
  mux-token-id: "<from Mux dashboard>"
  mux-token-secret: "<from Mux dashboard>"
  mux-webhook-secret: "<generated, verify webhook signatures>"
```

### 7.2 HTTPRoute (Webhook)

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: wigowago-api
spec:
  hostnames: ["wigowago-api.verse4.pet"]
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /webhooks/mux
      backendRefs:
        - name: wigowago-api
          port: 80
```

---

## 8. 成本模型

### Phase 1 (测试期，小规模)

假设: 月上传 200 个视频 × 平均 1 分钟 = 200 分钟
月播放 5,000 次 × 平均 1 分钟 = 5,000 分钟

| 项目 | 用量 | 单价 | 月成本 |
|------|------|------|--------|
| 编码 | 200 分钟 | $0 (免费) | $0 |
| 存储 | 200 分钟 | $0.0024/分/月 | $0.48 |
| 传输 | 5,000 分钟 | 免费额度内 (100K/月) | $0 |
| **总计** | | | **$0.48/月** |

### Phase 2 (生产期，中等规模)

假设: 月上传 10,000 个视频 × 平均 2 分钟 = 20,000 分钟
月播放 500,000 次 × 平均 2 分钟 = 1,000,000 分钟

| 项目 | 用量 | 单价 | 月成本 |
|------|------|------|--------|
| 编码 | 20,000 分钟 | $0 (免费) | $0 |
| 存储 | 20,000 分钟 | $0.0024/分 | $48 |
| 冷存储折扣 | (30天未看 × 60%) | -$15 | |
| 传输 | 1,000,000 - 100,000 免费 = 900,000 | $0.0008/分 | $720 |
| **总计** | | | **~$753/月** |

---

## 9. 实现计划

### Phase 1: 基础集成 (2 周)

- [ ] WigoWago API: Mux SDK 集成 + Direct Upload 端点
- [ ] WigoWago API: Mux Webhook 处理
- [ ] Regional DB: 新增 video 列迁移
- [ ] Flutter App: 视频上传 → Mux → 播放流程
- [ ] 测试: 端到端上传→转码→播放

### Phase 2: 完善 (1 周)

- [ ] 视频错误状态处理 + 重试
- [ ] 用户删除时清理 Mux assets
- [ ] GDPR: EU 用户跨边境同意流程
- [ ] Mux Data 分析接入

### Phase 3: 优化 (后续)

- [ ] `smart` 编码模式 (per-title, 省 30% 带宽)
- [ ] 视频压缩客户端预处理 (上传前降分辨率)
- [ ] 自建转码方案评估 (当月传 >50TB 时)

---

## 10. 数据驻留与 GDPR 深度分析

### 10.1 Mux 的数据驻留问题

Mux 的视频存储基于 GCP，**不支持用户选择存储区域**。其 DPA 提到的 Frankfurt (eu-central-1) 仅用于 Mux Data 分析的匿名化处理，不是视频文件的存储位置。

这意味着所有 EU 用户的视频内容都会传输到美国存储，需要：
- 用户明确同意跨边境传输
- 依赖 Standard Contractual Clauses (SCCs)
- 审计日志记录每次传输

### 10.2 api.video — EU 数据驻留的替代方案

| 维度 | api.video | Mux |
|------|-----------|-----|
| **EU 存储** | ✅ 支持，免费 | ❌ 不支持（全部在美国） |
| **公司总部** | 法国 (Roubaix) | 美国 (San Francisco) |
| **编码** | 免费（ASIC 硬件加速） | 免费（Just-in-Time） |
| **存储成本** | $0.00285/分钟/月 | $0.0024/分钟/月 |
| **传输成本** | $0.0017/分钟 | $0.0008/分钟 |
| **CDN** | Fastly (140+ POPs) | Fastly + Akamai (Multi-CDN) |
| **API 成熟度** | 中等 | 行业领先 |
| **分析能力** | 基础 | Mux Data (行业领先) |
| **典型客户** | Paris 2024 Olympics, Mercedes | Spotify, Uscreen |

### 10.3 推荐方案：双服务商策略

```
EU 用户 ──▶ api.video (EU 区域) ──▶ 数据留在欧洲
NA 用户 ──▶ Mux (US 区域) ──▶ 更低传输成本 + 更强分析
```

**理由**：
1. **GDPR 合规最简化** — EU 用户数据不出欧洲，不需要 SCCs 和额外同意
2. **成本可控** — api.video EU 存储/传输单价略高，但 EU 用户量初期有限
3. **灵活性** — 两个服务商都是 API-first，未来迁移成本低
4. **风险分散** — 不依赖单一供应商

### 10.4 WigoWago API 适配设计

```typescript
// 根据用户区域选择视频服务商
function getVideoProvider(userRegion: string): VideoProvider {
  if (userRegion === 'eu') {
    return new ApiVideoProvider({
      apiKey: process.env.APIVIDEO_KEY,
      region: 'eu', // 存储在欧盟
    });
  }
  return new MuxProvider({
    tokenId: process.env.MUX_TOKEN_ID,
    tokenSecret: process.env.MUX_TOKEN_SECRET,
  });
}

// 统一的 VideoProvider 接口
interface VideoProvider {
  createUploadUrl(postId: number, filename: string): Promise<UploadResult>;
  handleWebhook(event: WebhookEvent): Promise<void>;
  deleteAsset(assetId: string): Promise<void>;
  getPlaybackUrl(assetId: string): string;
}
```

**数据库不需要额外改动** — `mux_asset_id` 列改为通用的 `video_asset_id VARCHAR(64)`，`mux_upload_id` 改为 `video_provider VARCHAR(16)` 记录使用了哪个服务商。

---

## 11. 风险和缓解

| 风险 | 影响 | 缓解 |
|------|------|------|
| Mux 宕机 | 无法上传/播放视频 | 降级: 仍支持图片帖子, 视频显示 "暂不可用" |
| Mux webhook 丢失 | Post 永远处于 processing 状态 | 定时轮询 Mux API 检查 upload 状态 |
| 欧盟数据驻留 | GDPR 合规 | 用户同意 + 审计日志 (已有框架) |
| 成本超支 | 播放量超预期 | 设置 Mux 用量告警 + 每月审计 |

---

## 11. 对现有 Global Sync 的变更总结

| 组件 | 变更 | 工作量 |
|------|------|--------|
| `global_post_index.media_urls` | 无需改动，已支持 | 0 |
| GDPR Checker | 无需改动，TIER_2 规则适用 | 0 |
| Gossip 广播 | 无需改动，URL 是字符串 | 0 |
| 迁移文件 | 可选新增 video 列 | 10 分钟 |
| CrossSync Event Payload | 可选新增 video 字段 | 30 分钟 |

**结论: Global Sync Service 基本不需要改动。** 视频 URL 作为普通的 media URL 处理即可。
