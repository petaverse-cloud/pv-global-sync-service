# WigoWago Flutter App - Global Sync Service 整合技术文档

> **版本**: V2.1
> **日期**: 2026-04-17
> **目标**: 为 Flutter App 开发者提供跨集群内容同步的完整技术指引

---

## 0. 环境与部署规划

### 0.1 当前架构说明

**暂时没有独立的 Staging 环境。** SEA 和 EU 集群同时承担测试和生产角色。

### 0.2 环境演进

```
阶段一: 测试验证 (当前)
┌─────────────────────────────────────────────────────┐
│  SEA 集群 (DEVOPS)     EU 集群 (EU)                 │
│  wigowago-api.         api-eu.                      │
│  verse4.pet            wigowago.com                 │
│        │                      │                     │
│        └──── Gossip ──────────┘                     │
│         双向同步验证中                               │
└─────────────────────────────────────────────────────┘

阶段二: 生产上线 (测试验收后)
┌─────────────────────────────────────────────────────┐
│  SEA 集群         EU 集群 (EU Prod)    NA 集群      │
│  保持测试         域名不变             新部署        │
│  wigowago-api.    api-eu.            api-na.        │
│  verse4.pet       wigowago.com       wigowago.com   │
│        │               │                   │        │
│        └── Gossip ────┼── Gossip ────────┘        │
│                       │                             │
│                EU 正式成为                          │
│                欧洲生产集群                         │
└─────────────────────────────────────────────────────┘
```

### 0.3 域名汇总

| 集群 | 当前用途 | API 域名 | 测试验收后 |
|------|---------|----------|-----------|
| SEA (DEVOPS) | 测试 | `wigowago-api.verse4.pet` | 保持测试 |
| EU | 测试 | `api-eu.wigowago.com` | **EU 生产** (域名不变) |
| NA | 待部署 | `api-na.wigowago.com` | **NA 生产** (新部署) |

### 0.4 Flutter App 环境配置

App 根据构建环境选择 API 地址：

```dart
class ApiConfig {
  static const _envs = {
    'test': {
      'sea': 'https://wigowago-api.verse4.pet',  // SEA 测试
      'eu':  'https://api-eu.wigowago.com',       // EU 测试
    },
    'prod': {
      'eu': 'https://api-eu.wigowago.com',        // EU 生产 (域名不变)
      'na': 'https://api-na.wigowago.com',        // NA 生产 (新上线)
    },
  };

  static String get({required String env, required String region}) {
    return _envs[env]![region]!;
  }
}

// 构建命令
// 测试 - SEA
flutter build apk --dart-define=APP_ENV=test --dart-define=APP_REGION=sea

// 测试 - EU
flutter build apk --dart-define=APP_ENV=test --dart-define=APP_REGION=eu

// 生产 - EU (验收后)
flutter build apk --dart-define=APP_ENV=prod --dart-define=APP_REGION=eu

// 生产 - NA (验收后)
flutter build apk --dart-define=APP_ENV=prod --dart-define=APP_REGION=na
```

---

## 1. 架构概览

### 1.1 核心原则

**Flutter App 不直接连接 Global Sync Service。**

App 始终只与 **区域 WigoWago API** 通信，跨集群内容由 API 内部合并后透明返回。

```
┌──────────────┐         ┌────────────────────────┐         ┌─────────────────────┐
│  Flutter App │ ──HTTPS──▶ │  Regional API (Node.js) │ ──HTTP──▶ │ Global Sync Service │
│  (SEA/NA/EU) │            │  api-na/sea/eu.        │            │  (Go, port 8080)  │
│              │ ◀───────── │  wigowago.com          │ ◀───────── │  :8080            │
└──────────────┘            └────────────────────────┘            └─────────────────────┘
                                    │                                      │
                                    ▼                                      ▼
                            Regional DB (PostgreSQL)              Global Index (PostgreSQL)
                            Posts, Users, Pets                    Cross-region post index
```

### 1.2 数据流向

```
用户创建帖子 (SEA 集群)
  → SEA Regional DB 写入
  → SEA Global Sync Service 索引
  → Gossip 同步到 EU Global Sync Service
  → EU Global Index 记录

App 请求帖子列表 (EU 集群)
  → EU API 查询 EU Regional DB (本地帖子)
  → EU API 调用 EU Global Sync /feed (SEA 帖子)
  → 合并、去重、按时间排序
  → 返回完整 Feed
```

---

## 2. 区域路由

### 2.1 编译时区域绑定

App 必须在构建时确定目标区域：

```bash
# NA 构建
flutter build ios --dart-define=APP_REGION=na
flutter build apk --dart-define=APP_REGION=na

# EU 构建
flutter build ios --dart-define=APP_REGION=eu
flutter build apk --dart-define=APP_REGION=eu
```

### 2.2 请求头注入

所有 HTTP 请求必须携带 `X-App-Region` 头：

```dart
class RegionalApiClient extends DioMixin {
  final String region;
  final String baseUrl;

  RegionalApiClient({required this.region, required this.baseUrl}) {
    options.baseUrl = baseUrl;
    interceptors.add(InterceptorsWrapper(
      onRequest: (options, handler) {
        options.headers['X-App-Region'] = region;
        return handler.next(options);
      },
    ));
  }
}

// 使用
final api = RegionalApiClient(
  region: String.fromEnvironment('APP_REGION', defaultValue: 'na'),
  baseUrl: ApiConfig.get(
    env: String.fromEnvironment('APP_ENV', defaultValue: 'prod'),
    region: String.fromEnvironment('APP_REGION', defaultValue: 'na'),
  ),
);
```

### 2.3 区域不匹配处理 (409 WRONG_REGION)

当用户区域与 API 区域不匹配时：

```json
HTTP/1.1 409 Conflict
Content-Type: application/json

{
  "code": "WRONG_REGION",
  "message": "User belongs to a different region",
  "data": {
    "userRegion": "na",
    "redirectUrl": "https://api-na.wigowago.com",
    "migrationSupported": true
  }
}
```

App 处理逻辑：

```dart
class RegionMismatchHandler {
  static Future<void> handle(DioException error) async {
    if (error.response?.statusCode != 409) return;

    final data = error.response?.data['data'] as Map<String, dynamic>;
    final userRegion = data['userRegion'] as String;
    final redirectUrl = data['redirectUrl'] as String;

    // kDebugMode: 允许调试时覆盖
    if (kDebugMode) {
      final override = await showDialog<String>(
        context: navigatorKey.currentContext!,
        builder: (_) => RegionOverrideDialog(
          currentRegion: 'eu',
          userRegion: userRegion,
        ),
      );
      if (override != null) return; // 用户选择忽略
    }

    // 生产环境: 提示用户前往正确的 App Store
    await showDialog(
      context: navigatorKey.currentContext!,
      builder: (_) => RegionMismatchDialog(
        userRegion: userRegion,
        redirectUrl: redirectUrl,
      ),
    );
  }
}
```

---

## 3. Feed API 使用

### 3.1 获取帖子列表 (自动合并跨集群内容)

```dart
/// GET /posts
///
/// 响应包含本地 + 跨集群帖子，自动去重和排序。
/// App 无需感知数据来源。
Future<PostListResponse> getPosts({
  int limit = 20,
  String? cursor,
}) async {
  final response = await api.get('/posts', queryParameters: {
    'limit': limit.clamp(1, 100),
    if (cursor != null) 'cursor': cursor,
  });
  return PostListResponse.fromJson(response.data);
}

class PostListResponse {
  final List<Post> posts;
  final String? nextCursor;
  final bool hasMore;
  final PostMetadata metadata;

  factory PostListResponse.fromJson(Map<String, dynamic> json) {
    return PostListResponse(
      posts: (json['posts'] as List)
          .map((e) => Post.fromJson(e))
          .toList(),
      nextCursor: json['nextCursor'],
      hasMore: json['hasMore'] ?? false,
      metadata: PostMetadata.fromJson(json['metadata'] ?? {}),
    );
  }
}

class Post {
  final int postId;
  final int authorId;
  final String authorRegion;   // "sea", "eu", "na"
  final String contentPreview;
  final List<String> hashtags;
  final List<String> mediaUrls; // CDN URLs, 跨区域渲染无需额外请求
  final int likesCount;
  final int commentsCount;
  final String createdAt;
}
```

### 3.2 Feed 类型

Global Sync Service 支持三种 feed 类型，由 API 内部调用：

| feedType | 说明 | TTL | 使用场景 |
|----------|------|-----|----------|
| `following` | 关注的人的帖子 | 5 min | "关注" Tab |
| `global` | 所有公开帖子 | 15 min | "发现" Tab |
| `trending` | 24h 内热度最高的帖子 | 1 min | "热门" Tab |

App 通过 `/posts?feedType=xxx` 切换（如果 API 支持该参数）。

### 3.3 分页

```dart
class PostPagingController {
  final posts = ValueNotifier<List<Post>>([]);
  String? _cursor;
  bool _loading = false;
  bool _hasMore = true;

  Future<void> loadNext() async {
    if (_loading || !_hasMore) return;
    _loading = true;

    try {
      final response = await getPosts(cursor: _cursor);
      posts.value = [...posts.value, ...response.posts];
      _cursor = response.nextCursor;
      _hasMore = response.hasMore;
    } finally {
      _loading = false;
    }
  }
}
```

---

## 4. 降级与容错

### 4.1 Global Sync 不可用

当 Global Sync Service 宕机时，API 自动降级为仅返回 Regional DB 帖子：

```dart
/// App 侧无需特殊处理。
/// 如果 Global Sync 不可用：
///   - GET /posts 仍返回 200
///   - 响应中只包含本地帖子 (authorRegion == 当前区域)
///   - hasMore 可能为 false
///
/// 可选：检测跨区域帖子数量，在 UI 提示
void checkFeedCompleteness(List<Post> posts, String currentRegion) {
  final crossRegionPosts = posts.where((p) => p.authorRegion != currentRegion).length;
  if (crossRegionPosts == 0 && posts.isNotEmpty) {
    // 可能 Global Sync 不可用，但不一定是错误
    log('No cross-region posts in feed (may be degraded mode)');
  }
}
```

### 4.2 网络超时

```dart
class RegionalApiClient extends DioMixin {
  RegionalApiClient({required this.region, required this.baseUrl}) {
    options = BaseOptions(
      baseUrl: baseUrl,
      connectTimeout: const Duration(seconds: 10),
      receiveTimeout: const Duration(seconds: 30),
    );
    // ...
  }
}
```

---

## 5. GDPR 合规 (App 侧)

### 5.1 媒体文件

- 媒体文件存储在 **用户所在区域** 的 Blob Storage
- App 收到的是 **CDN URL**，直接加载即可
- CDN 跨区域可达，无需额外鉴权

```dart
// mediaUrls 中的 CDN URL 可直接使用
Image.network(
  post.mediaUrls[0],
  errorBuilder: (_, __, ___) => const Icon(Icons.broken_image),
)
```

### 5.2 用户同意

- 用户首次登录时展示跨区域数据同步说明
- 用户可在设置中关闭跨集群可见性
- 关闭后帖子 `visibility` 设为 `REGIONAL`，不会进入 Global Index

---

## 6. 错误码参考

| HTTP 状态码 | 错误码 | 说明 | App 处理 |
|-------------|--------|------|----------|
| 400 | `BAD_REQUEST` | 请求格式错误 | 检查请求参数 |
| 401 | `UNAUTHORIZED` | Token 过期/无效 | 重新登录 |
| 409 | `WRONG_REGION` | 用户区域不匹配 | 提示用户切换 App Store |
| 500 | `INTERNAL_ERROR` | 服务器内部错误 | 重试，3 次后显示错误页 |
| 503 | `SERVICE_UNAVAILABLE` | Global Sync 降级中 | 正常，本地帖子仍可用 |

---

## 7. 测试验证

### 7.1 跨集群内容可见性

```dart
test('cross-region posts appear in feed', () async {
  // 前提: EU 集群已部署，gossip 正常
  final api = RegionalApiClient(region: 'eu', baseUrl: 'https://api-eu.wigowago.com');

  final response = await api.get('/posts', queryParameters: {'limit': 50});
  final seaPosts = response.posts.where((p) => p.authorRegion == 'sea').toList();

  expect(seaPosts.isNotEmpty, isTrue, reason: 'Should see SEA posts in EU feed');
});
```

### 7.2 降级模式验证

```dart
test('feed returns local posts when global sync is down', () async {
  // 模拟 Global Sync 不可用（可通过修改 API 环境变量实现）
  final response = await getPosts();

  // 仍应返回 200，但只包含本地帖子
  expect(response.posts.isNotEmpty, isTrue);
  // 所有帖子都来自本地区域
  for (final post in response.posts) {
    expect(post.authorRegion, equals('eu'));
  }
});
```

---

## 8. 已知限制

| 限制 | 说明 | 缓解 |
|------|------|------|
| Feed 延迟 | 跨区域帖子有 3-5 秒同步延迟 | 实时性要求高的场景用 WebSocket |
| 分页限制 | 单次最多 100 条 | 使用 cursor 分批加载 |
| 媒体 CDN | 跨区域 CDN URL 可达但速度取决于地域 | 使用 Azure CDN 多区域加速 |
| 关注 Feed | EU 集群 Regional DB 无用户数据时降级为 pull mode | 部署 wigowago-api 到 EU 后解决 |

---

## 9. 附录: 完整 API 端点列表

| 端点 | 方法 | 说明 | 需要认证 |
|------|------|------|----------|
| `/posts` | GET | 获取帖子列表 (含跨集群) | 是 |
| `/posts` | POST | 创建帖子 | 是 |
| `/posts/:id` | GET | 获取帖子详情 | 否 |
| `/posts/:id` | PUT | 更新帖子 | 是 |
| `/posts/:id` | DELETE | 删除帖子 | 是 |
| `/auth/email/verify` | POST | 验证码登录 | 否 |
| `/auth/logout` | POST | 登出 | 是 |
| `/health` | GET | 服务健康检查 | 否 |

### Base URLs

| 阶段 | 集群 | 角色 | API 域名 |
|------|------|------|----------|
| 测试期 (当前) | SEA | 测试 | `https://wigowago-api.verse4.pet` |
| 测试期 (当前) | EU | 测试 | `https://api-eu.wigowago.com` |
| 生产期 (验收后) | EU | EU 生产 | `https://api-eu.wigowago.com` (域名不变) |
| 生产期 (验收后) | NA | NA 生产 | `https://api-na.wigowago.com` (新部署) |

> **注意**:
> - App 只与 WigoWago API 通信，不直接连接 Global Sync Service。
> - Global Sync 是 API 的后端基础设施，跨集群数据同步对 App 完全透明。
> - EU 集群在测试验收后直接转为生产，**域名保持不变**，App 无需更新配置。
> - SEA 集群保持测试角色，不作为生产环境。
