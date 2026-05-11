# Global-Sync-Service — Deep Route-to-DB Audit

> Generated: 2026-05-11 · 14 endpoints · 5 services · 1 DB · Full call chain trace

---

## New Bug Found

### 🔴 CRITICAL: `GetGlobalPosts` References Dropped `post_id` Column

**Location**: `internal/service/global_index.go:409`

```sql
SELECT post_id, COALESCE(post_slug, 0), author_uid, ...  -- BUG: post_id dropped!
FROM global_post_index
```

**Migration 013** (`013_drop_post_id_rename_author_uid.sql`):
```sql
ALTER TABLE global_post_index DROP COLUMN IF EXISTS post_id;
```

**Impact**: `GET /feed/{userId}?type=global` → `feedGenerator.GetFeed("global")` → `GetGlobalPosts()` → **runtime SQL error "column post_id does not exist"**. The Global feed is completely broken.

**Why it was missed**:
- `GetPostsFromAuthors` and `GetTrendingPosts` were both correctly fixed to use `COALESCE(post_slug, 0) AS post_uid`
- `GetGlobalPosts` was the **only query missed** in the `post_id` cleanup
- Unit test: `feed_generator_test.go` uses `mockFeedIndex` which never calls real `GetGlobalPosts`
- E2E test: does not test the feed endpoint at all

---

## Complete Endpoint Audit

### 1. `POST /sync/content` → HandleSync

| Layer | Call | DB Query | 
|---|---|---|
| Router | `syncHandler.HandleSync` | — |
| Handler | JSON decode → validate → `processEvent()` | — |
| Handler | `eventLog.IsProcessed(eventID)` | Redis: `EXISTS` / DB: `SELECT EXISTS FROM sync_event_log` |
| Handler | `gdprChecker.Check(event)` | In-memory (6 rules, no DB) |
| Handler | `auditSvc.Log()` | DB: `INSERT INTO sync_audit_log` |
| Handler | `routeEvent()` → dispatch | See per-event-type below |
| Handler | `eventLog.MarkProcessed()` | Redis: `SET` + DB: `INSERT INTO sync_event_log` |
| Handler | `crossSync.Broadcast()` (async) | Outbound HTTP to peer regions |

**Route branches**:
| `POST_CREATED` | `indexSvc.InsertPost()` → DB: `INSERT INTO global_post_index ... ON CONFLICT (post_slug) DO UPDATE` | ✅ |
| `POST_CREATED` | `feedGenerator.HandleNewPost()` → DB: `SELECT followers_count` + Redis `ZADD` | ✅ |
| `POST_UPDATED` | `indexSvc.UpdatePost()` → DB: `UPDATE global_post_index WHERE post_slug = $1` | ✅ |
| `POST_DELETED` | `indexSvc.DeletePost()` → DB: `DELETE FROM global_post_index WHERE post_slug = $1` | ✅ |
| `POST_DELETED` | `feedGenerator.HandleDeletedPost()` → (no-op currently) | ✅ |
| `POST_STATS_UPDATED` | `handleStatsUpdated()` → **DB: `SELECT ... FROM posts WHERE uid = $1`** (Regional DB) + `UpdateStats()` → Global DB | ✅ |
| `TAG_CREATED/UPDATED` | `tagIndexSvc.UpsertTag()` → DB: `INSERT INTO global_tag_index ... ON CONFLICT (tag_uid) DO UPDATE` | ✅ |
| `TAG_DELETED` | `tagIndexSvc.DeleteTag()` → DB: `DELETE FROM global_tag_index WHERE tag_uid = $1` | ✅ |
| `TAG_STATS_UPDATED` | `tagIndexSvc.UpdateStats()` → DB: `UPDATE global_tag_index SET post_count = ...` | ✅ |

**Unit tests**: 26 (full pipeline + all branches + idempotent + GDPR + validation)  
**E2E test**: ✅ Post CRUD lifecycle

---

### 2. `POST /sync/cross-sync` → HandleCrossSync

Identical flow to `/sync/content`, except `source = "cross_sync"` which skips `crossSync.Broadcast()` (cross-sync events are NOT re-broadcast).

**Unit tests**: 3 (success + GDPR denied + route error)  
**E2E test**: Not directly tested (used internally by cross-sync mechanism)

---

### 3. `GET /index/posts/{uid}` → HandleGetPost

| Layer | Call | DB Query |
|---|---|---|
| Router | `syncHandler.HandleGetPost` | — |
| Handler | `chi.URLParam("uid")` → `parseInt64()` | — |
| Service | `indexSvc.GetPost()` | DB: `SELECT ... FROM global_post_index WHERE post_slug = $1` |
| Handler | 200/404/500 response | — |

**Unit tests**: 3 (found, not-found, invalid uid)  
**E2E test**: ✅ Create → Read → Verify content

---

### 4. `GET /index/posts/uid/{uid}` → HandleGetPostByUid

Identical to HandleGetPost but calls `indexSvc.GetPostByUid()` instead.

**Note**: `GetPostByUid` and `GetPost` are **identical methods** — both query `WHERE post_slug = $1`. Duplicate code.

**Unit tests**: 2 (found, invalid uid)  
**E2E test**: Not tested separately

---

### 5. `POST /index/users/check` → HandleCheckUser

| Layer | Call | DB Query |
|---|---|---|
| Handler | JSON decode → `indexSvc.FindRegionByEmailHash()` | DB: `SELECT uid, region, email_hash FROM users_global_index WHERE email_hash = $1` |
| Handler | Returns `{exists: bool, region: string}` | — |

**Unit tests**: 0 for handler; service-level tests cover FindRegionByEmailHash  
**E2E test**: ✅ Upsert → Check exists → Check non-existent

---

### 6. `POST /index/users/upsert` → HandleUpsertUser

| Layer | Call | DB Query |
|---|---|---|
| Handler | JSON decode → `indexSvc.UpsertUserIndex()` | DB: `INSERT INTO users_global_index ... ON CONFLICT (uid) DO UPDATE` |
| Handler | `broadcastUserIndex()` → HTTP POST to peers (fire-and-forget, with retry) | — |

**Unit tests**: 2 service-level (insert, nil emailHash)  
**E2E test**: ✅ Upsert → Verify via check

---

### 7. `GET /index/users/all` → HandleGetAllUsers

| Layer | Call | DB Query |
|---|---|---|
| Handler | `indexSvc.GetAllUserIndexEntries()` | DB: `SELECT uid, email_hash, region FROM users_global_index` |

**Used by**: `UserIndexReconciler` to fetch peer's user list for comparison.  
**Unit tests**: 1 service-level  
**E2E test**: Not tested directly

---

### 8. `GET /index/user/region` → HandleGetUserRegion

| Layer | Call | DB Query |
|---|---|---|
| Handler | `?uid=` → `indexSvc.FindRegionByUID()` | DB: `SELECT region FROM users_global_index WHERE uid = $1` |

**Unit tests**: 2 service-level (found, not-found)  
**E2E test**: ✅ Upsert → Get region

---

### 9. `GET /feed/{userId}` → HandleGetFeed

| Layer | Call | DB/Redis Query |
|---|---|---|
| Handler | Parse `?type=` and `?limit=` | — |
| Service | `feedGenerator.GetFeed()` | — |
| Service | Branch: `type=following` → Redis `ZREVRANGEBYSCORE` | Redis: cached feed |
| Service | Branch: `type=following` (cache miss) → DB: `SELECT follower_uid FROM user_follows` → `GetPostsFromAuthors()` | DB: `SELECT ... FROM global_post_index WHERE author_uid = ANY($1)` |
| Service | Branch: `type=global` → Redis cache check → **`GetGlobalPosts()`** | DB: `SELECT post_id, ...` 🔴 **BROKEN** |
| Service | Branch: `type=trending` → Redis cache check → `GetTrendingPosts()` | DB: `SELECT ... FROM global_post_index ORDER BY engagement` |

**Unit tests**: 10 (push/pull mode, cache hit, routing, TTLs)  
**E2E test**: ❌ Not tested at all

---

### 10. `GET /index/tags/search` → HandleSearchTags

| Layer | Call | DB Query |
|---|---|---|
| Handler | `?keyword=` + `?limit=` → `tagIndexSvc.SearchTags()` | DB: `SELECT ... FROM global_tag_index WHERE name ILIKE $1 LIMIT $2` |

**Unit tests**: 1 handler + 2 service-level  
**E2E test**: ✅

---

### 11. `GET /index/tags/popular` → HandlePopularTags

| Layer | Call | DB Query |
|---|---|---|
| Handler | `?limit=` → `tagIndexSvc.GetPopularTags()` | DB: `SELECT ... FROM global_tag_index ORDER BY post_count DESC LIMIT $1` |

**Unit tests**: 1 handler + 1 service-level  
**E2E test**: ✅

---

### 12. `GET /index/tags/{tagUid}` → HandleGetTag

| Layer | Call | DB Query |
|---|---|---|
| Handler | `chi.URLParam("tagUid")` → `tagIndexSvc.GetTagByUID()` | DB: `SELECT ... FROM global_tag_index WHERE tag_uid = $1` |

**Unit tests**: 2 handler (found, not-found) + 2 service-level  
**E2E test**: ✅

---

### 13. `GET /index/tags/{tagUid}/regions` → HandleGetTagRegions

| Layer | Call | DB Query |
|---|---|---|
| Handler | `chi.URLParam("tagUid")` → `tagIndexSvc.GetRegionsForTag()` | DB: `SELECT DISTINCT home_region FROM global_tag_index WHERE tag_uid = $1` |

**Unit tests**: 1 handler + 2 service-level  
**E2E test**: ✅

---

### 14. `GET /health`, `/health/live`, `/health/ready`

| Handler | DB Query |
|---|---|
| `handleHealth` | `SELECT 1` — basic connectivity |
| `handleLiveness` | No DB, just 200 |
| `handleReadiness` | `SELECT 1` + Redis `PING` |

**Unit tests**: 9  
**E2E test**: ✅

---

## Background: `UserIndexReconciler.Run`

| Layer | Call | DB/HTTP |
|---|---|---|
| Reconciler | `fetchPeerEntries()` → HTTP GET `/index/users/all` from peer | Peer HTTP |
| Reconciler | DB: `SELECT uid, email_hash, region FROM users_global_index` | Local DB |
| Reconciler | Compare: peer entries not in local → `UpsertUserIndex()` | DB: `INSERT ... ON CONFLICT (uid) DO UPDATE` |

**Unit tests**: 10  
**E2E test**: Not tested

---

## Bug Summary

| # | Bug | Found By | Location | Severity |
|---|---|---|---|---|
| 1 | `InsertPost` referenced dropped `post_id` | E2E | `global_index.go:64` | 🔴 Fixed |
| 2 | `global_tag_index` table migration missing | E2E | Schema | 🔴 Fixed |
| 3 | nil `TagPostCount` dereference → crash | Unit | `sync_handler.go:312` | 🟡 Fixed |
| 4 | GDPR blocking DELETE + TAG events | E2E analysis | `gdpr_checker.go` | 🟡 Fixed |
| **5** | **`GetGlobalPosts` references dropped `post_id`** | **Deep Audit** | **`global_index.go:409`** | **🔴 OPEN** |

---

## Single-Action Fix

```go
// internal/service/global_index.go:409
// BEFORE (broken):
query := `
    SELECT post_id, COALESCE(post_slug, 0), author_uid, content_preview, ...
`

// AFTER (fix):
query := `
    SELECT COALESCE(post_slug, 0) AS post_uid, author_uid, content_preview, ...
`
```

And remove the `&p.PostID` from `rows.Scan()` on line 426 (replace with a dummy `_` variable or update `GlobalIndexPost` struct since `PostID` field is dead).

---

## Endpoint Coverage Matrix

| Endpoint | Unit Test | E2E Test | DB Correct? |
|---|---|---|---|
| `POST /sync/content` | 26 | ✅ | ✅ |
| `POST /sync/cross-sync` | 3 | — | ✅ |
| `GET /index/posts/{uid}` | 3 | ✅ | ✅ |
| `GET /index/posts/uid/{uid}` | 2 | — | ✅ |
| `POST /index/users/check` | 2 svc | ✅ | ✅ |
| `POST /index/users/upsert` | 2 svc | ✅ | ✅ |
| `GET /index/users/all` | 1 svc | — | ✅ |
| `GET /index/user/region` | 2 svc | ✅ | ✅ |
| **`GET /feed/{userId}`** | 10 | ❌ | **🔴 BROKEN** |
| `GET /index/tags/search` | 3 | ✅ | ✅ |
| `GET /index/tags/popular` | 2 | ✅ | ✅ |
| `GET /index/tags/{tagUid}` | 4 | ✅ | ✅ |
| `GET /index/tags/{tagUid}/regions` | 3 | ✅ | ✅ |
| `GET /health` | 9 | ✅ | ✅ |

**Score**: 13/14 endpoints fully verified. 1 endpoint broken (`/feed` with type=global). E2E test coverage has one gap: the feed endpoint is completely untested at the E2E level.
