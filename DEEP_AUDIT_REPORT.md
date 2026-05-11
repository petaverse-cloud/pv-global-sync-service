# Global Sync Service — Deep Audit Report
## All 14 Endpoints: Route → Handler → Service → DB Column Verification

**Repository**: /Users/wesley/workspace/Petaverse/src/pv-global-sync-service
**Date**: 2026-05-11
**Auditor**: Hermes Agent

---

## EFFECTIVE SCHEMA (after all 14 migrations applied)

### global_post_index (001→007→009→010→012→013)
```
post_slug          BIGINT          (was post_id PK; migrated to post_slug)
author_uid         BIGINT          (was author_id; migrated from author_slug)
author_region      VARCHAR(16)
content_preview    TEXT
visibility         VARCHAR(20)
hashtags           TEXT[]
mentions           BIGINT[]
media_urls         TEXT[]
likes_count        INTEGER
comments_count     INTEGER
shares_count       INTEGER
views_count        INTEGER
gdpr_compliant     BOOLEAN
user_consent       BOOLEAN
data_category      VARCHAR(20)
created_at         TIMESTAMPTZ
synced_at          TIMESTAMPTZ
updated_at         TIMESTAMPTZ
author_slug        BIGINT          (007)
author_nickname    VARCHAR(100)    (007)
author_avatar_url  VARCHAR(255)    (007)
```

### users_global_index (005→006→008→011; fully rebuilt by 011)
```
uid         BIGINT PRIMARY KEY
region      VARCHAR(16) NOT NULL
email_hash  VARCHAR(64)
created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
```

### user_feed (002)
```
user_uid    BIGINT NOT NULL
post_uid    BIGINT NOT NULL
feed_type   VARCHAR(20) NOT NULL
score       DECIMAL(10,6) NOT NULL
created_at  TIMESTAMPTZ NOT NULL
expires_at  TIMESTAMPTZ
PRIMARY KEY (user_uid, feed_type, post_uid)
```

### sync_event_log (004)
```
event_id       VARCHAR(64) PRIMARY KEY
event_type     VARCHAR(32) NOT NULL
source_region  VARCHAR(16) NOT NULL
processed_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
status         VARCHAR(20) NOT NULL DEFAULT 'pending'
error_message  TEXT
```

### cross_border_audit_log (003)
```
log_id           BIGSERIAL PRIMARY KEY
event_id         VARCHAR(64) NOT NULL
timestamp        TIMESTAMPTZ NOT NULL DEFAULT NOW()
data_subject_id  BIGINT NOT NULL
source_region    VARCHAR(16) NOT NULL
target_region    VARCHAR(16) NOT NULL
data_type        VARCHAR(50) NOT NULL
legal_basis      VARCHAR(100)
user_consent     BOOLEAN DEFAULT false
status           VARCHAR(20) NOT NULL
metadata         JSONB
```

### global_tag_index (014)
```
tag_uid        BIGINT PRIMARY KEY
name           VARCHAR(50) NOT NULL
home_region    VARCHAR(10) NOT NULL
category_uid   BIGINT
post_count     BIGINT DEFAULT 0
last_active_at TIMESTAMPTZ
created_at     TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
updated_at     TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
```

---

## PER-ENDPOINT TRACE

### 1. GET /health
**Route**: server.go:209 `r.Get("/health", ...)`
**Handler**: inline `handleHealth` (server.go:243)
**Service**: N/A — direct infrastructure checks
**DB**: `db.Ping()` + `redis.Ping()`
**Error codes**: 200 (ok) / 503 (degraded)
**Columns**: N/A
**VERDICT**: ✅ VERIFIED — no DB columns involved.

---

### 2. GET /health/live
**Route**: server.go:212 `r.Get("/health/live", handleLiveness)`
**Handler**: `handleLiveness` (server.go:270)
**Service**: N/A
**DB**: None
**Error codes**: Always 200
**Columns**: N/A
**VERDICT**: ✅ VERIFIED

---

### 3. GET /health/ready
**Route**: server.go:213 `r.Get("/health/ready", ...)`
**Handler**: inline `handleReadiness` (server.go:277)
**Service**: N/A — infrastructure checks
**DB**: `db.Ping()` + `redis.Ping()`
**Error codes**: 200 (ready) / 503 (not ready: database|redis)
**Columns**: N/A
**VERDICT**: ✅ VERIFIED

---

### 4. POST /sync/content → HandleSync
**Route**: server.go:218 `r.Post("/sync/content", syncHandler.HandleSync)`
**Handler**: `HandleSync` (sync_handler.go:107)
  - Validates: method=POST (405), JSON body (400), eventId+eventType required (400)
  - Pipeline: `processEvent` → idempotency → GDPR → audit → routeEvent → cross-sync broadcast
**Service chain** (via processEvent → routeEvent):
  - eventLog.IsProcessed → MarkProcessed
  - gdprChecker.Check → auditSvc.Log
  - Dispatching by eventType to:
    - POST_CREATED → indexSvc.InsertPost → feedGenerator.HandleNewPost
    - POST_UPDATED → indexSvc.UpdatePost
    - POST_DELETED → indexSvc.DeletePost → feedGenerator.HandleDeletedPost
    - POST_STATS_UPDATED → handleStatsUpdated → indexSvc.UpdateStats
    - TAG_CREATED/TAG_UPDATED → tagIndexSvc.UpsertTag
    - TAG_DELETED → tagIndexSvc.DeleteTag
    - TAG_STATS_UPDATED → tagIndexSvc.UpdateStats

**DB column verification**:

*InsertPost* (global_index.go:59):
```sql
INSERT INTO global_post_index (
    post_slug, author_uid, author_region, content_preview, visibility,
    hashtags, mentions, media_urls, likes_count, comments_count, shares_count, views_count,
    gdpr_compliant, user_consent, data_category, created_at, synced_at,
    author_nickname, author_avatar_url
) VALUES ($1..$15)
ON CONFLICT (post_slug) DO UPDATE SET ...
```
Schema columns: post_slug✅ author_uid✅ author_region✅ content_preview✅ visibility✅ hashtags✅ mentions✅ media_urls✅ likes_count✅ comments_count✅ shares_count✅ views_count✅ gdpr_compliant✅ user_consent✅ data_category✅ created_at✅ synced_at✅ author_nickname✅ author_avatar_url✅

*UpdatePost* (global_index.go:126):
```sql
UPDATE global_post_index SET content_preview, visibility, hashtags, media_urls,
    updated_at=NOW(), synced_at=NOW() WHERE post_slug = $5
```
All columns exist. ✅

*DeletePost* (global_index.go:164):
```sql
DELETE FROM global_post_index WHERE post_slug = $1
```
✅

*UpdateStats* (global_index.go:181):
```sql
UPDATE global_post_index SET likes_count, comments_count, shares_count,
    views_count, updated_at=NOW() WHERE post_slug = $5
```
All columns exist. ✅

*UpsertTag* (global_tag_index.go:34):
```sql
INSERT INTO global_tag_index (tag_uid, name, home_region, category_uid,
    post_count, last_active_at, created_at, updated_at)
VALUES ($1..$5) ON CONFLICT (tag_uid) DO UPDATE SET ...
```
All columns exist. ✅

*DeleteTag / UpdateTagStats* — all columns verified. ✅

*EventLog.IsProcessed* (event_log.go:57):
```sql
SELECT EXISTS(SELECT 1 FROM sync_event_log WHERE event_id = $1 AND status = 'processed')
```
Columns: event_id✅ status✅

*EventLog.MarkProcessed* (event_log.go:77):
```sql
INSERT INTO sync_event_log (event_id, event_type, source_region, status, error_message)
VALUES ($1..$5) ON CONFLICT (event_id) DO UPDATE SET status, error_message
```
Columns: event_id✅ event_type✅ source_region✅ status✅ error_message✅
(processed_at uses DEFAULT NOW() — fine)

*AuditLog* (gdpr_checker.go:180):
```sql
INSERT INTO cross_border_audit_log (event_id, data_subject_id, source_region,
    target_region, data_type, legal_basis, user_consent, status, metadata)
VALUES ($1..$9)
```
All columns exist. ✅

**Error codes**: 405 (wrong method), 400 (bad JSON/missing fields), 202 (accepted), 500 (processing error)
**VERDICT**: ✅ ALL COLUMNS VERIFIED. ⚠️ See CRITICAL ISSUE #1 below (ON CONFLICT post_slug).

---

### 5. POST /sync/cross-sync → HandleCrossSync
**Route**: server.go:219 `r.Post("/sync/cross-sync", syncHandler.HandleCrossSync)`
**Handler**: `HandleCrossSync` (sync_handler.go:144)
**Service chain**: Same as #4 but source="cross_sync" (no cross-sync broadcast)
**DB**: Same as #4
**Error codes**: Same as #4
**VERDICT**: ✅ Identical DB verification to endpoint #4.

---

### 6. GET /index/posts/{uid} → HandleGetPost
**Route**: server.go:222 `r.Get("/index/posts/{uid}", syncHandler.HandleGetPost)`
**Handler**: `HandleGetPost` (sync_handler.go:181)
  - Validates: uid param (400 if missing/invalid)
  - Service: indexSvc.GetPost
  - Error: 200 (with post), 404 (not found), 500 (db error)
**DB query** (global_index.go:197):
```sql
SELECT COALESCE(post_slug,0), author_uid, author_region, content_preview, visibility,
       hashtags, mentions, COALESCE(array_to_string(media_urls,','),'') AS media_urls_str,
       likes_count, comments_count, shares_count, views_count,
       gdpr_compliant, user_consent, data_category, created_at, synced_at,
       author_nickname, author_avatar_url
FROM global_post_index WHERE post_slug = $1
```
Columns: post_slug✅ author_uid✅ author_region✅ content_preview✅ visibility✅ hashtags✅ mentions✅ media_urls✅ likes_count✅ comments_count✅ shares_count✅ views_count✅ gdpr_compliant✅ user_consent✅ data_category✅ created_at✅ synced_at✅ author_nickname✅ author_avatar_url✅

**Error codes**: 400 (missing/invalid uid), 404 (not found), 500 (db error)
**VERDICT**: ✅ ALL COLUMNS VERIFIED. ⚠️ See MODERATE ISSUE #2 (mentions type).

---

### 7. GET /index/posts/uid/{uid} → HandleGetPostByUid
**Route**: server.go:223 `r.Get("/index/posts/uid/{uid}", syncHandler.HandleGetPostByUid)`
**Handler**: `HandleGetPostByUid` (sync_handler.go:361)
**Service**: indexSvc.GetPostByUid (alias for GetPost)
**DB**: Identical to #6
**Error codes**: 400/404/500
**VERDICT**: ✅ Same as #6. Duplicate endpoint (both use post_slug).

---

### 8. POST /index/users/check → HandleCheckUser
**Route**: server.go:227 `r.Post("/index/users/check", userIndexHandler.HandleCheckUser)`
**Handler**: `HandleCheckUser` (user_index_handler.go:48)
  - Validates: method=POST (405), JSON body (400), emailHash required (400)
  - Service: indexSvc.FindRegionByEmailHash
**DB query** (global_index.go:518):
```sql
SELECT region FROM users_global_index WHERE email_hash = $1
```
Columns: region✅ email_hash✅

**Error codes**: 405, 400, 500
**VERDICT**: ✅ VERIFIED.

---

### 9. POST /index/users/upsert → HandleUpsertUser
**Route**: server.go:228 `r.Post("/index/users/upsert", userIndexHandler.HandleUpsertUser)`
**Handler**: `HandleUpsertUser` (user_index_handler.go:82)
  - Validates: method=POST (405), JSON body (400), uid+region required (400)
  - Service: indexSvc.UpsertUserIndex
  - Fire-and-forget broadcast to peers
**DB query** (global_index.go:501):
```sql
INSERT INTO users_global_index (uid, region, email_hash)
VALUES ($1, $2, $3)
ON CONFLICT (uid) DO UPDATE SET region = $2, email_hash = $3, updated_at = NOW()
```
Columns: uid✅ region✅ email_hash✅ updated_at✅

**Error codes**: 405, 400, 200, 500
**VERDICT**: ✅ VERIFIED.

---

### 10. GET /index/users/all → HandleGetAllUsers
**Route**: server.go:229 `r.Get("/index/users/all", userIndexHandler.HandleGetAllUsers)`
**Handler**: `HandleGetAllUsers` (user_index_handler.go:236)
  - Validates: method=GET (405)
  - Service: indexSvc.GetAllUserIndexEntries
**DB query** (global_index.go:542):
```sql
SELECT uid, email_hash, region FROM users_global_index ORDER BY uid
```
Columns: uid✅ email_hash✅ region✅

**Error codes**: 405, 200, 500
**VERDICT**: ✅ VERIFIED.

---

### 11. GET /index/user/region → HandleGetUserRegion
**Route**: server.go:230 `r.Get("/index/user/region", userIndexHandler.HandleGetUserRegion)`
**Handler**: `HandleGetUserRegion` (user_index_handler.go:201)
  - Validates: method=GET (405), uid query param (400 if missing/invalid)
  - Service: indexSvc.FindRegionByUID
**DB query** (global_index.go:526):
```sql
SELECT region FROM users_global_index WHERE uid = $1
```
Columns: region✅ uid✅

**Error codes**: 405, 400, 404, 500
**VERDICT**: ✅ VERIFIED.

---

### 12. GET /feed/{userId} → HandleGetFeed
**Route**: server.go:233 `r.Get("/feed/{userId}", feedHandler.HandleGetFeed)`
**Handler**: `HandleGetFeed` (feed_handler.go:38)
  - Validates: userId param (400 if invalid), feedType (default: "following"), limit (1-100, default 20)
  - Service: generator.GetFeed → getFollowingFeed/getGlobalFeed/getTrendingFeed
    - Redis ZSET cache check first
    - On miss: Regional DB queries + Global Index queries

**DB queries (feed_generator.go)**:

*Following feed — Regional DB*:
```sql
SELECT following_uid FROM user_follows WHERE follower_uid = $1
```
(Regional DB table — not in global index migrations)

*Following feed — Global Index* (global_index.go:374):
```sql
SELECT COALESCE(post_slug,0) AS post_uid, author_uid, content_preview,
       likes_count, comments_count, shares_count, views_count,
       created_at, author_nickname, author_avatar_url
FROM global_post_index WHERE author_uid = ANY($1) ORDER BY created_at DESC LIMIT $2
```
Columns: post_slug✅ author_uid✅ content_preview✅ likes_count✅ comments_count✅ shares_count✅ views_count✅ created_at✅ author_nickname✅ author_avatar_url✅

*Global feed* (global_index.go:406):
```sql
SELECT COALESCE(post_slug,0), author_uid, content_preview,
       likes_count, comments_count, shares_count, views_count,
       created_at, author_nickname, author_avatar_url
FROM global_post_index ORDER BY created_at DESC LIMIT $1
```
Same columns. ✅

*Trending feed* (global_index.go:437):
```sql
SELECT COALESCE(post_slug,0) AS post_uid, author_uid, content_preview,
       likes_count, comments_count, shares_count, views_count,
       created_at, author_nickname, author_avatar_url
FROM global_post_index WHERE created_at > NOW() - INTERVAL '24 hours'
ORDER BY (likes_count + comments_count*2 + shares_count*3) DESC LIMIT $1
```
Same columns. ✅

**Error codes**: 400 (invalid userId), 500 (db error)
**VERDICT**: ✅ ALL COLUMNS VERIFIED.

---

### 13. GET /index/tags/search → HandleSearchTags
**Route**: server.go:236 `r.Get("/index/tags/search", syncHandler.HandleSearchTags)`
**Handler**: `HandleSearchTags` (sync_handler.go:395)
  - Query: keyword, limit (default 20)
  - Service: tagIndexSvc.SearchTags
**DB query** (global_tag_index.go:98):
```sql
SELECT tag_uid, name, home_region, category_uid, post_count, last_active_at, created_at, updated_at
FROM global_tag_index WHERE name ILIKE '%' || $1 || '%'
ORDER BY post_count DESC, name ASC LIMIT $2
```
Columns: tag_uid✅ name✅ home_region✅ category_uid✅ post_count✅ last_active_at✅ created_at✅ updated_at✅

**Error codes**: 500
**VERDICT**: ✅ VERIFIED.

---

### 14. GET /index/tags/popular → HandlePopularTags
**Route**: server.go:237 `r.Get("/index/tags/popular", syncHandler.HandlePopularTags)`
**Handler**: `HandlePopularTags` (sync_handler.go:410)
  - Query: limit (default 20)
  - Service: tagIndexSvc.GetPopularTags
**DB query** (global_tag_index.go:117):
```sql
SELECT tag_uid, name, home_region, category_uid, post_count, last_active_at, created_at, updated_at
FROM global_tag_index ORDER BY post_count DESC, name ASC LIMIT $1
```
Same columns as #13. ✅

**Error codes**: 500
**VERDICT**: ✅ VERIFIED.

---

### 15. GET /index/tags/{tagUid} → HandleGetTag
**Route**: server.go:238 `r.Get("/index/tags/{tagUid}", syncHandler.HandleGetTag)`
**Handler**: `HandleGetTag` (sync_handler.go:424)
  - Validates: tagUid param (400 if invalid)
  - Service: tagIndexSvc.GetTagByUID
**DB query** (global_tag_index.go:135):
```sql
SELECT tag_uid, name, home_region, category_uid, post_count, last_active_at, created_at, updated_at
FROM global_tag_index WHERE tag_uid = $1
```
Same columns. ✅

**Error codes**: 400, 404, 500
**VERDICT**: ✅ VERIFIED.

---

### 16. GET /index/tags/{tagUid}/regions → HandleGetTagRegions
**Route**: server.go:239 `r.Get("/index/tags/{tagUid}/regions", syncHandler.HandleGetTagRegions)`
**Handler**: `HandleGetTagRegions` (sync_handler.go:447)
  - Validates: tagUid param (400 if invalid)
  - Service: tagIndexSvc.GetRegionsForTag
**DB query** (global_tag_index.go:155):
```sql
SELECT DISTINCT home_region FROM global_tag_index WHERE tag_uid = $1
```
Columns: home_region✅ tag_uid✅

**Error codes**: 400, 500
**VERDICT**: ✅ VERIFIED.

---

## ISSUES FOUND

### 🔴 CRITICAL

**ISSUE #1: Missing unique constraint on `post_slug` — ON CONFLICT will fail at runtime**

- **Location**: `global_index.go:74` — `ON CONFLICT (post_slug) DO UPDATE`
- **Root cause**: Migration 013 drops `post_id` (the original PRIMARY KEY from migration 001). Migration 009 adds `post_slug BIGINT` but only creates a regular INDEX, not a UNIQUE constraint or PRIMARY KEY. There is NO migration that promotes `post_slug` to PRIMARY KEY or adds a UNIQUE constraint.
- **Impact**: PostgreSQL will reject the `ON CONFLICT (post_slug)` clause at runtime with error: `there is no unique or exclusion constraint matching the ON CONFLICT specification`. All InsertPost operations will fail.
- **Also**: Migration 013's `ALTER TABLE global_post_index DROP COLUMN IF EXISTS post_id` will fail if `post_id` is still the PK, because dropping a PK column requires CASCADE. The `IF EXISTS` only prevents errors when the column doesn't exist — it does NOT bypass the PK constraint requirement.
- **Fix needed**: Add a migration (015) that:
  ```sql
  ALTER TABLE global_post_index ADD CONSTRAINT global_post_index_pkey PRIMARY KEY (post_slug);
  ```
  OR create a unique index: `CREATE UNIQUE INDEX IF NOT EXISTS idx_gpi_post_slug_unique ON global_post_index(post_slug);`
  BEFORE migration 013 runs, or ensure 013 uses CASCADE and a new PK is added.

---

### 🟡 MODERATE

**ISSUE #2: `mentions` column type mismatch — BIGINT[] scanned as TEXT[]**

- **Location**: `global_index.go:197` (GetPost), `global_index.go:284` (pgtypeArray.Scan)
- **Detail**: Schema defines `mentions BIGINT[]` (migration 001). The `pgtypeArray` type implements `Scan` and parses values as `[]string`, not `[]int64`. The model struct `GlobalPostIndex.Mentions` is `[]int64`. PostgreSQL transmits arrays as text (`{123,456}`), so this technically works, but the scanner doesn't validate that each element is a valid int64.
- **Impact**: LOW — strings will be parsed as int64 when used, but malformed data won't be caught at scan time.

**ISSUE #3: `favorites_count` stored as `shares_count` — semantic mismatch**

- **Location**: `sync_handler.go:324` (handleStatsUpdated) and `consumer/sync_consumer.go:207` (handleStatsUpdated)
- **Detail**: Regional DB's `posts` table has `favorites_count`. The global index has `shares_count`. The code reads `favorites_count` from regional DB and writes it as `shares_count` in global index. Comment says "Note: Regional DB has favorites_count, not shares_count."
- **Impact**: MODERATE — Data is semantically different (favorites ≠ shares). If a consumer of the global index interprets `shares_count` as actual shares, the data is misleading.

**ISSUE #4: `home_region` column width mismatch — VARCHAR(10) vs VARCHAR(16)**

- **Location**: `global_tag_index` table (migration 014) vs all other tables
- **Detail**: `global_tag_index.home_region` is `VARCHAR(10)`. Every other region column across the schema uses `VARCHAR(16)` (global_post_index.author_region, users_global_index.region, sync_event_log.source_region, cross_border_audit_log.source_region/target_region). If a region code like "us-west-2" (10 chars) or longer is used, it will fit in other tables but get truncated in tag_index.
- **Impact**: LOW currently (codes are short: "EU", "NA", "SEA"), but fragility if region codes grow.

**ISSUE #5: Consumer doesn't handle TAG events**

- **Location**: `consumer/sync_consumer.go:170` (routeEvent)
- **Detail**: The HTTP handler's `routeEvent` dispatches TAG_CREATED, TAG_UPDATED, TAG_DELETED, TAG_STATS_UPDATED to tagIndexSvc. The consumer's `routeEvent` only handles POST_CREATED, POST_UPDATED, POST_DELETED, POST_STATS_UPDATED — tag events fall through to `default: return nil` (silently skipped).
- **Impact**: MODERATE — If tag events are ever sent via RocketMQ (not just HTTP), they will be silently dropped. This may be intentional (tags only sync via HTTP), but there's no comment explaining this design choice.

---

### 🟢 MINOR / OBSERVATIONS

**OBS #6**: `sync_event_log` INSERT in `MarkProcessed` doesn't set `processed_at` — relies on DEFAULT NOW(). Fine, but `ON CONFLICT DO UPDATE` also doesn't touch `processed_at`, meaning the original timestamp is preserved on retry. This is correct behavior.

**OBS #7**: Error code 202 (Accepted) is used for POST /sync/content and /sync/cross-sync. This is appropriate for async processing but HTTP 202 is typically for requests "accepted for processing but not yet completed." The response body suggests the event is already processed synchronously. Consider 200 or 201 for synchronous success.

**OBS #8**: Duplicate endpoint functionality — `/index/posts/{uid}` and `/index/posts/uid/{uid}` both resolve to the same `GetPost` query. The separate `HandleGetPostByUid` is just an alias. Consider consolidating.

**OBS #9**: `author_slug` column exists in `global_post_index` (from migration 007) but is never read or written by any service code. Dead column.

---

## SUMMARY

| Category | Count |
|----------|-------|
| Endpoints audited | 14 (+ 2 tag sub-endpoints = 16 routes) |
| DB queries verified | 21 |
| Columns cross-referenced | All match ✅ |
| CRITICAL issues | 1 (missing post_slug unique constraint) |
| MODERATE issues | 4 (mentions type, favorites/shares mismatch, home_region width, consumer tag gap) |
| MINOR observations | 4 |

**Overall**: All SQL column references are correct against the migration schema. The single critical issue — missing PRIMARY KEY / UNIQUE constraint on `post_slug` — will cause runtime failures on any INSERT/UPSERT operation. This must be resolved before deployment.
