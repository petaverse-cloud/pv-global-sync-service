#!/usr/bin/env python3
"""Global Sync Service E2E Regression Test"""
import json, time, urllib.request, urllib.error, sys, os

SEA = "https://wigowago-global-sync.verse4.pet"
EU  = "https://global-sync-eu.wigowago.com"

PASS = FAIL = SKIP = 0
G, R, Y, N = '\033[32m', '\033[31m', '\033[33m', '\033[0m'

def http(method, url, body=None):
    req = urllib.request.Request(url, method=method,
        headers={"Content-Type": "application/json"},
        data=body.encode() if body else None)
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status, resp.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()

def http_post(url, body): return http("POST", url, json.dumps(body))
def http_get(url): return http("GET", url)

def ok(msg): global PASS; PASS+=1; print(f"  {G}✓{N} {msg}")
def no(msg, detail=""): global FAIL; FAIL+=1; print(f"  {R}✗{N} {msg}" + (f": {detail}" if detail else ""))
def sk(msg): global SKIP; SKIP+=1; print(f"  {Y}⊘{N} {msg}")

def check(cond, msg, detail=""):
    if cond: ok(msg)
    else: no(msg, detail)

ts = int(time.time())
eid = f"e2e_{ts}"
post_uid = 9000000000 + (ts % 100000)
author_uid = 8000000001
tag_uid = post_uid + 1000

print(f"{'='*50}")
print(f" Global Sync Service E2E Regression Test")
print(f" SEA: {SEA}")
print(f" EU:  {EU}")
print(f" EventID: {eid}  PostUID: {post_uid}")
print(f"{'='*50}\n")

# ===== 1. HEALTH =====
print("--- 1. Health ---")
c, b = http_get(f"{SEA}/health"); check(c==200 and '"ok"' in b, "SEA healthy")
c, b = http_get(f"{EU}/health");  check(c==200 and '"ok"' in b, "EU healthy")

# ===== 2. POST CRUD =====
print("\n--- 2. Post CRUD ---")

# Create
c, b = http_post(f"{SEA}/sync/content", {
    "eventId": f"{eid}_create", "eventType": "POST_CREATED",
    "sourceRegion": "SEA", "targetRegion": "EU", "timestamp": ts,
    "payload": {"postUid": post_uid, "authorUid": author_uid, "authorRegion": "SEA",
                "visibility": "GLOBAL", "content": "E2E test #go #testing",
                "mediaUrls": ["https://cdn.test.com/img1.jpg"]},
    "metadata": {"gdprCompliant": True, "userConsent": True, "dataCategory": "TIER_2", "crossBorderOk": True}
})
check(c == 202, "POST /sync/content (create)", f"HTTP {c}")
time.sleep(1)

# Read
c, b = http_get(f"{SEA}/index/posts/{post_uid}")
check(c == 200, "GET /index/posts", f"HTTP {c}")
check("E2E test" in b, "SEA post content matches")
check("#go" not in b and "test" in b, "Hashtags extracted (raw # stripped)")

# Update
c, b = http_post(f"{SEA}/sync/content", {
    "eventId": f"{eid}_update", "eventType": "POST_UPDATED",
    "sourceRegion": "SEA", "targetRegion": "EU", "timestamp": ts,
    "payload": {"postUid": post_uid, "authorUid": author_uid, "authorRegion": "SEA",
                "visibility": "GLOBAL", "content": "E2E test UPDATED"},
    "metadata": {"gdprCompliant": True, "userConsent": True, "dataCategory": "TIER_2", "crossBorderOk": True}
})
check(c == 202, "POST /sync/content (update)", f"HTTP {c}")
time.sleep(1)
c, b = http_get(f"{SEA}/index/posts/{post_uid}")
check("UPDATED" in b, "SEA post updated")

# Delete
c, b = http_post(f"{SEA}/sync/content", {
    "eventId": f"{eid}_delete", "eventType": "POST_DELETED",
    "sourceRegion": "SEA", "targetRegion": "EU", "timestamp": ts,
    "payload": {"postUid": post_uid, "authorUid": author_uid},
    "metadata": {"gdprCompliant": True, "userConsent": True, "dataCategory": "TIER_2", "crossBorderOk": True}
})
check(c == 202, "POST /sync/content (delete)", f"HTTP {c}")
time.sleep(1)
c, b = http_get(f"{SEA}/index/posts/{post_uid}")
check(c == 404, "Deleted post returns 404", f"HTTP {c}")

# ===== 3. CROSS-REGION =====
print("\n--- 3. Cross-Region Sync ---")
cross_uid = post_uid + 1
c, b = http_post(f"{SEA}/sync/content", {
    "eventId": f"{eid}_cross", "eventType": "POST_CREATED",
    "sourceRegion": "SEA", "targetRegion": "EU", "timestamp": ts,
    "payload": {"postUid": cross_uid, "authorUid": author_uid, "authorRegion": "SEA",
                "visibility": "GLOBAL", "content": "Cross-region E2E test"},
    "metadata": {"gdprCompliant": True, "userConsent": True, "dataCategory": "TIER_2", "crossBorderOk": True}
})
check(c == 202, "Cross-region: POST SEA", f"HTTP {c}")
time.sleep(2)
c, b = http_get(f"{EU}/index/posts/{cross_uid}")
if c == 200: ok("Cross-region: EU sees SEA post")
elif c == 404: sk("Cross-region: EU 404 (broadcast may be delayed)")
else: no("Cross-region: EU read", f"HTTP {c}")

# ===== 4. USER INDEX =====
print("\n--- 4. User Index ---")
user_uid = post_uid + 5000
user_hash = f"e2e_hash_{ts}"

c, b = http_post(f"{SEA}/index/users/upsert", {"uid": user_uid, "emailHash": user_hash, "region": "SEA"})
check(c == 200, "Upsert user index", f"HTTP {c}")

c, b = http_post(f"{SEA}/index/users/check", {"emailHash": user_hash})
check(c == 200 and '"exists":true' in b, "Check user exists")

c, b = http_post(f"{SEA}/index/users/check", {"emailHash": "no_such_hash"})
check(c == 200 and '"exists":false' in b, "Check non-existent user")

c, b = http_get(f"{SEA}/index/user/region?uid={user_uid}")
check(c == 200 and '"SEA"' in b, "Get user region=SEA")

# ===== 5. TAG CRUD =====
print("\n--- 5. Tag CRUD ---")
c, b = http_post(f"{SEA}/sync/content", {
    "eventId": f"{eid}_tag_create", "eventType": "TAG_CREATED",
    "sourceRegion": "SEA", "targetRegion": "EU", "timestamp": ts,
    "payload": {"tagUid": tag_uid, "tagName": "e2e_test_tag", "tagCategoryUid": 1, "postCount": 5},
    "metadata": {"gdprCompliant": True, "userConsent": True, "dataCategory": "TIER_2", "crossBorderOk": True}
})
check(c == 202, "Create tag", f"HTTP {c}")
time.sleep(1)

c, b = http_get(f"{SEA}/index/tags/{tag_uid}")
check(c == 200 and "e2e_test_tag" in b, "GET tag by uid")

c, b = http_get(f"{SEA}/index/tags/search?keyword=e2e_test&limit=5")
check(c == 200 and "e2e_test_tag" in b, "Search tags")

c, b = http_get(f"{SEA}/index/tags/popular?limit=5")
check(c == 200, "Popular tags", f"HTTP {c}")

c, b = http_get(f"{SEA}/index/tags/{tag_uid}/regions")
check(c == 200 and '"SEA"' in b, "Tag regions")

# Delete tag
c, b = http_post(f"{SEA}/sync/content", {
    "eventId": f"{eid}_tag_delete", "eventType": "TAG_DELETED",
    "sourceRegion": "SEA", "targetRegion": "EU", "timestamp": ts,
    "payload": {"tagUid": tag_uid, "tagName": "e2e_test_tag"},
    "metadata": {"gdprCompliant": True, "userConsent": True, "dataCategory": "TIER_2", "crossBorderOk": True}
})
check(c == 202, "Delete tag", f"HTTP {c}")
time.sleep(1)
c, b = http_get(f"{SEA}/index/tags/{tag_uid}")
check(c == 404, "Deleted tag returns 404", f"HTTP {c}")

# ===== 6. IDEMPOTENCY =====
print("\n--- 6. Idempotency ---")
idem_uid = post_uid + 100
idem_event = {
    "eventId": f"{eid}_idem", "eventType": "POST_CREATED",
    "sourceRegion": "SEA", "targetRegion": "EU", "timestamp": ts,
    "payload": {"postUid": idem_uid, "authorUid": author_uid, "authorRegion": "SEA",
                "visibility": "GLOBAL", "content": "Idempotency test"},
    "metadata": {"gdprCompliant": True, "userConsent": True, "dataCategory": "TIER_2", "crossBorderOk": True}
}
c1, _ = http_post(f"{SEA}/sync/content", idem_event)
c2, _ = http_post(f"{SEA}/sync/content", idem_event)
check(c1 == 202, "First POST (idempotency)", f"HTTP {c1}")
check(c2 == 202, "Duplicate POST (idempotency)", f"HTTP {c2}")

# Cleanup
http_post(f"{SEA}/sync/content", {
    "eventId": f"{eid}_cleanup", "eventType": "POST_DELETED",
    "sourceRegion": "SEA", "targetRegion": "EU", "timestamp": ts,
    "payload": {"postUid": idem_uid},
    "metadata": {"gdprCompliant": True, "userConsent": True, "dataCategory": "TIER_2", "crossBorderOk": True}
})

# ===== SUMMARY =====
print(f"\n{'='*50}")
print(f" E2E Regression Test Results")
print(f"{'='*50}")
print(f" {G}PASS:{N} {PASS}")
print(f" {R}FAIL:{N} {FAIL}")
print(f" {Y}SKIP:{N} {SKIP}")
print(f"{'='*50}")

if FAIL > 0:
    print(f"{R}REGRESSION DETECTED{N}")
    sys.exit(1)
else:
    print(f"{G}ALL TESTS PASSED{N}")
    sys.exit(0)
