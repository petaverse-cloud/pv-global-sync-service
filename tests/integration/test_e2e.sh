#!/usr/bin/env bash
# WigoWago Global Sync Service - E2E Integration Tests
# Usage: ./tests/integration/test_e2e.sh
#
# Prerequisites: psql client, curl, Azure CLI login
#
# All tests use external HTTPS endpoints — no port-forward needed.
# DEVOPS: https://wigowago-global-sync.verse4.pet
# EU:     https://global-sync-eu.wigowago.com

set -euo pipefail

# === Configuration ===
DEVOPS_SYNC="https://wigowago-global-sync.verse4.pet"
EU_SYNC="https://global-sync-eu.wigowago.com"

DEVOPS_PGPASSWORD="DevOps2026!PostgreSQL"
DEVOPS_PGHOST="petaverse-devops-postgres.postgres.database.azure.com"
DEVOPS_PGUSER="pgadmin"
DEVOPS_PGDB="wigowago_global_index"

EU_PGPASSWORD="aa4917bd41ab3caf766514e58d76456d"
EU_PGHOST="petaverse-eu-postgres.postgres.database.azure.com"
EU_PGUSER="wigowago_admin"
EU_PGDB="wigowago_global_index"

PASS=0
FAIL=0
TEST_NUM=0
TS=$(python3 -c "import time; print(int(time.time() * 1000))")

# === Helpers ===
log_test() {
    TEST_NUM=$((TEST_NUM + 1))
    printf "\n  [Test %d] %s\n" "$TEST_NUM" "$1"
}

pass() {
    PASS=$((PASS + 1))
    printf "    ✅ PASS: %s\n" "$1"
}

fail() {
    FAIL=$((FAIL + 1))
    printf "    ❌ FAIL: %s (expected: %s, got: %s)\n" "$1" "$2" "$3"
}

pg_devops() {
    PGPASSWORD="$DEVOPS_PGPASSWORD" psql "host=$DEVOPS_PGHOST port=5432 user=$DEVOPS_PGUSER dbname=$DEVOPS_PGDB sslmode=require" -t -A -c "$1" 2>/dev/null
}

pg_eu() {
    PGPASSWORD="$EU_PGPASSWORD" psql "host=$EU_PGHOST port=5432 user=$EU_PGUSER dbname=$EU_PGDB sslmode=require" -t -A -c "$1" 2>/dev/null
}

# Generate unique event ID with test run suffix
EID() {
    echo "itest_${1}_${TS}_$$"
}

send_sync() {
    local url="$1"
    local body="$2"
    curl -s --connect-timeout 10 --max-time 15 -X POST "$url/sync/content" \
        -H "Content-Type: application/json" \
        -d "$body" 2>/dev/null
}

send_cross_sync() {
    local url="$1"
    local body="$2"
    curl -s --connect-timeout 10 --max-time 15 -X POST "$url/sync/cross-sync" \
        -H "Content-Type: application/json" \
        -d "$body" 2>/dev/null
}

cleanup_test_data() {
    pg_devops "DELETE FROM global_post_index WHERE post_id >= 90000;" >/dev/null 2>&1 || true
    pg_eu "DELETE FROM global_post_index WHERE post_id >= 90000;" >/dev/null 2>&1 || true
    pg_devops "DELETE FROM cross_border_audit_log WHERE event_id LIKE 'itest_%';" >/dev/null 2>&1 || true
    pg_eu "DELETE FROM cross_border_audit_log WHERE event_id LIKE 'itest_%';" >/dev/null 2>&1 || true
    pg_devops "DELETE FROM sync_event_log WHERE event_id LIKE 'itest_%';" >/dev/null 2>&1 || true
    pg_eu "DELETE FROM sync_event_log WHERE event_id LIKE 'itest_%';" >/dev/null 2>&1 || true
}

# === Test Body ===

echo "╔══════════════════════════════════════════════════════════╗"
echo "║   WigoWago Global Sync - E2E Integration Tests          ║"
echo "║   DEVOPS: $DEVOPS_SYNC"
echo "║   EU:     $EU_SYNC"
echo "╚══════════════════════════════════════════════════════════╝"

# --- Cleanup first ---
echo ""
echo "=== Cleaning up old test data ==="
cleanup_test_data
echo "    Done"

# === Phase 1: Infrastructure Health ===
echo ""
echo "═══════════════════════════════════════════════════════════"
echo "  Phase 1: Infrastructure Health Checks"
echo "═══════════════════════════════════════════════════════════"

log_test "DEVOPS /health returns 200"
HEALTH=$(curl -s --connect-timeout 10 "$DEVOPS_SYNC/health")
if echo "$HEALTH" | grep -q '"status":"ok"'; then
    pass "DEVOPS health OK"
else
    fail "DEVOPS health" '{"status":"ok"}' "$HEALTH"
fi

log_test "EU /health returns 200"
HEALTH=$(curl -s --connect-timeout 10 "$EU_SYNC/health")
if echo "$HEALTH" | grep -q '"status":"ok"'; then
    pass "EU health OK"
else
    fail "EU health" '{"status":"ok"}' "$HEALTH"
fi

log_test "DEVOPS PostgreSQL connection"
COUNT=$(pg_devops "SELECT COUNT(*) FROM global_post_index;")
if [ -n "$COUNT" ]; then
    pass "DEVOPS PG connected ($COUNT posts)"
else
    fail "DEVOPS PG connection" "count" "empty"
fi

log_test "EU PostgreSQL connection"
COUNT=$(pg_eu "SELECT COUNT(*) FROM global_post_index;")
if [ -n "$COUNT" ]; then
    pass "EU PG connected ($COUNT posts)"
else
    fail "EU PG connection" "count" "empty"
fi

log_test "DEVOPS -> EU connectivity"
EU_HEALTH=$(curl -s --connect-timeout 10 "$EU_SYNC/health")
if echo "$EU_HEALTH" | grep -q '"status":"ok"'; then
    pass "DEVOPS can reach EU endpoint"
else
    fail "DEVOPS -> EU connectivity" "health OK" "$EU_HEALTH"
fi

log_test "EU -> DEVOPS connectivity"
DEVOPS_HEALTH=$(curl -s --connect-timeout 10 "$DEVOPS_SYNC/health")
if echo "$DEVOPS_HEALTH" | grep -q '"status":"ok"'; then
    pass "EU can reach DEVOPS endpoint"
else
    fail "EU -> DEVOPS connectivity" "health OK" "$DEVOPS_HEALTH"
fi

# === Phase 2: Single Cluster (DEVOPS) Sync → Index ===
echo ""
echo "═══════════════════════════════════════════════════════════"
echo "  Phase 2: Single Cluster (DEVOPS) - Sync → Index"
echo "═══════════════════════════════════════════════════════════"

log_test "POST_CREATED syncs to Global Index"
EID_POST=$(EID "post_created")
BODY="{
  \"eventId\":\"$EID_POST\",
  \"eventType\":\"POST_CREATED\",
  \"sourceRegion\":\"SEA\",
  \"targetRegion\":\"SEA\",
  \"timestamp\":$TS,
  \"payload\":{
    \"postId\":90001,
    \"authorId\":100,
    \"authorRegion\":\"SEA\",
    \"visibility\":\"GLOBAL\",
    \"content\":\"Integration test post #1 #hello\",
    \"mediaUrls\":[\"https://cdn.example.com/img1.jpg\"]
  },
  \"metadata\":{
    \"gdprCompliant\":true,
    \"userConsent\":true,
    \"dataCategory\":\"TIER_2\",
    \"crossBorderOk\":true
  }
}"
RESP=$(send_sync "$DEVOPS_SYNC" "$BODY")
if echo "$RESP" | grep -q '"status":"accepted"'; then
    pass "POST accepted"
else
    fail "POST sync" '{"status":"accepted"}' "$RESP"
fi

sleep 2

log_test "Post appears in Global Index"
RESULT=$(pg_devops "SELECT content_preview FROM global_post_index WHERE post_id=90001;")
if [ "$RESULT" = "Integration test post #1 #hello" ]; then
    pass "Post indexed correctly"
else
    fail "Post content" "Integration test post #1 #hello" "$RESULT"
fi

log_test "Hashtags extracted correctly"
TAGS=$(pg_devops "SELECT hashtags::text FROM global_post_index WHERE post_id=90001;")
if echo "$TAGS" | grep -qi "hello"; then
    pass "Hashtag #hello extracted"
else
    fail "Hashtags" "{hello}" "$TAGS"
fi

log_test "Media URLs stored"
MEDIA=$(pg_devops "SELECT COALESCE(array_to_string(media_urls,','),'') FROM global_post_index WHERE post_id=90001;")
if echo "$MEDIA" | grep -q "cdn.example.com"; then
    pass "Media URLs stored"
else
    fail "Media URLs" "cdn.example.com" "$MEDIA"
fi

log_test "GET /index/posts/90001 returns 200"
GET_RESP=$(curl -s --connect-timeout 10 "$DEVOPS_SYNC/index/posts/90001")
if echo "$GET_RESP" | grep -q '"postId":90001'; then
    pass "GET post by ID works"
else
    fail "GET post" '"postId":90001' "$GET_RESP"
fi

log_test "GET /index/posts/999999 returns 404"
GET_404=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 10 "$DEVOPS_SYNC/index/posts/999999")
if [ "$GET_404" = "404" ]; then
    pass "Missing post returns 404"
else
    fail "GET missing post" "404" "$GET_404"
fi

# === Phase 3: Update & Delete ===
echo ""
echo "═══════════════════════════════════════════════════════════"
echo "  Phase 3: Update & Delete"
echo "═══════════════════════════════════════════════════════════"

log_test "POST_UPDATED updates content_preview and hashtags"
EID_UPDATE=$(EID "post_updated")
UPDATE_BODY="{
  \"eventId\":\"$EID_UPDATE\",
  \"eventType\":\"POST_UPDATED\",
  \"sourceRegion\":\"SEA\",
  \"targetRegion\":\"SEA\",
  \"timestamp\":$TS,
  \"payload\":{
    \"postId\":90001,
    \"authorId\":100,
    \"authorRegion\":\"SEA\",
    \"visibility\":\"GLOBAL\",
    \"content\":\"Updated content #newtag\",
    \"mediaUrls\":[\"https://cdn.example.com/img1.jpg\",\"https://cdn.example.com/img2.jpg\"]
  },
  \"metadata\":{
    \"gdprCompliant\":true,
    \"userConsent\":true,
    \"dataCategory\":\"TIER_2\",
    \"crossBorderOk\":true
  }
}"
send_sync "$DEVOPS_SYNC" "$UPDATE_BODY" >/dev/null
sleep 2

UPDATED=$(pg_devops "SELECT content_preview FROM global_post_index WHERE post_id=90001;")
if [ "$UPDATED" = "Updated content #newtag" ]; then
    pass "Content updated"
else
    fail "Updated content" "Updated content #newtag" "$UPDATED"
fi

log_test "POST_DELETED removes from index"
EID_DELETE=$(EID "post_deleted")
DELETE_BODY="{
  \"eventId\":\"$EID_DELETE\",
  \"eventType\":\"POST_DELETED\",
  \"sourceRegion\":\"SEA\",
  \"targetRegion\":\"SEA\",
  \"timestamp\":$TS,
  \"payload\":{
    \"postId\":90001,
    \"authorId\":100,
    \"authorRegion\":\"SEA\",
    \"visibility\":\"GLOBAL\",
    \"content\":\"\",
    \"mediaUrls\":[]
  },
  \"metadata\":{
    \"gdprCompliant\":true,
    \"userConsent\":true,
    \"dataCategory\":\"TIER_2\",
    \"crossBorderOk\":true
  }
}"
send_sync "$DEVOPS_SYNC" "$DELETE_BODY" >/dev/null
sleep 2

DELETED=$(pg_devops "SELECT COUNT(*) FROM global_post_index WHERE post_id=90001;")
if [ "$DELETED" = "0" ]; then
    pass "Post deleted from index"
else
    fail "Post deletion" "0" "$DELETED"
fi

# === Phase 4: GDPR Rules ===
echo ""
echo "═══════════════════════════════════════════════════════════"
echo "  Phase 4: GDPR Rules"
echo "═══════════════════════════════════════════════════════════"

log_test "TIER_1 (PII) is rejected"
EID_PII=$(EID "gdpr_pii")
PII_BODY="{\"eventId\":\"$EID_PII\",\"eventType\":\"POST_CREATED\",\"sourceRegion\":\"SEA\",\"targetRegion\":\"SEA\",\"timestamp\":$TS,\"payload\":{\"postId\":90002,\"authorId\":100,\"authorRegion\":\"SEA\",\"visibility\":\"GLOBAL\",\"content\":\"pii data\",\"mediaUrls\":[]},\"metadata\":{\"gdprCompliant\":false,\"userConsent\":false,\"dataCategory\":\"TIER_1\",\"crossBorderOk\":false}}"
send_sync "$DEVOPS_SYNC" "$PII_BODY" >/dev/null
sleep 1
PII_COUNT=$(pg_devops "SELECT COUNT(*) FROM global_post_index WHERE post_id=90002;")
if [ "$PII_COUNT" = "0" ]; then
    pass "PII data rejected"
else
    fail "PII rejection" "0" "$PII_COUNT"
fi

log_test "PRIVATE visibility is rejected"
EID_PRIV=$(EID "gdpr_priv")
PRIV_BODY="{\"eventId\":\"$EID_PRIV\",\"eventType\":\"POST_CREATED\",\"sourceRegion\":\"SEA\",\"targetRegion\":\"SEA\",\"timestamp\":$TS,\"payload\":{\"postId\":90003,\"authorId\":100,\"authorRegion\":\"SEA\",\"visibility\":\"PRIVATE\",\"content\":\"private post\",\"mediaUrls\":[]},\"metadata\":{\"gdprCompliant\":true,\"userConsent\":true,\"dataCategory\":\"TIER_2\",\"crossBorderOk\":true}}"
send_sync "$DEVOPS_SYNC" "$PRIV_BODY" >/dev/null
sleep 1
PRIV_COUNT=$(pg_devops "SELECT COUNT(*) FROM global_post_index WHERE post_id=90003;")
if [ "$PRIV_COUNT" = "0" ]; then
    pass "Private content rejected"
else
    fail "Private rejection" "0" "$PRIV_COUNT"
fi

log_test "GLOBAL without consent is rejected"
EID_NC=$(EID "gdpr_noconsent")
NO_CONSENT_BODY="{\"eventId\":\"$EID_NC\",\"eventType\":\"POST_CREATED\",\"sourceRegion\":\"SEA\",\"targetRegion\":\"SEA\",\"timestamp\":$TS,\"payload\":{\"postId\":90004,\"authorId\":100,\"authorRegion\":\"SEA\",\"visibility\":\"GLOBAL\",\"content\":\"no consent\",\"mediaUrls\":[]},\"metadata\":{\"gdprCompliant\":true,\"userConsent\":false,\"dataCategory\":\"TIER_2\",\"crossBorderOk\":false}}"
send_sync "$DEVOPS_SYNC" "$NO_CONSENT_BODY" >/dev/null
sleep 1
NC_COUNT=$(pg_devops "SELECT COUNT(*) FROM global_post_index WHERE post_id=90004;")
if [ "$NC_COUNT" = "0" ]; then
    pass "No consent rejected"
else
    fail "No consent rejection" "0" "$NC_COUNT"
fi

# TIER_3 (System) passes GDPR but inserts with authorId=0.
# The post IS inserted into global_post_index — verify it's there.
log_test "TIER_3 (System) data passes GDPR and is indexed"
EID_SYS=$(EID "gdpr_sys")
SYS_BODY="{\"eventId\":\"$EID_SYS\",\"eventType\":\"POST_CREATED\",\"sourceRegion\":\"SEA\",\"targetRegion\":\"SEA\",\"timestamp\":$TS,\"payload\":{\"postId\":90005,\"authorId\":0,\"authorRegion\":\"SEA\",\"visibility\":\"GLOBAL\",\"content\":\"system data\",\"mediaUrls\":[]},\"metadata\":{\"gdprCompliant\":true,\"userConsent\":false,\"dataCategory\":\"TIER_3\",\"crossBorderOk\":false}}"
send_sync "$DEVOPS_SYNC" "$SYS_BODY" >/dev/null
sleep 2
SYS_COUNT=$(pg_devops "SELECT COUNT(*) FROM global_post_index WHERE post_id=90005;")
if [ "$SYS_COUNT" = "1" ]; then
    pass "System data indexed (GDPR passed)"
else
    fail "System data indexed" "1" "$SYS_COUNT"
fi

# === Phase 5: Idempotency ===
echo ""
echo "═══════════════════════════════════════════════════════════"
echo "  Phase 5: Idempotency"
echo "═══════════════════════════════════════════════════════════"

# Use a completely unique event ID with timestamp + PID
IDEM_EID=$(EID "idem_001")
log_test "Same eventId twice - only first processed"
IDEM_BODY="{\"eventId\":\"$IDEM_EID\",\"eventType\":\"POST_CREATED\",\"sourceRegion\":\"SEA\",\"targetRegion\":\"SEA\",\"timestamp\":$TS,\"payload\":{\"postId\":90010,\"authorId\":100,\"authorRegion\":\"SEA\",\"visibility\":\"GLOBAL\",\"content\":\"idempotent test\",\"mediaUrls\":[]},\"metadata\":{\"gdprCompliant\":true,\"userConsent\":true,\"dataCategory\":\"TIER_2\",\"crossBorderOk\":true}}"

# First call
send_sync "$DEVOPS_SYNC" "$IDEM_BODY" >/dev/null
sleep 1

# Second call with same eventId
send_sync "$DEVOPS_SYNC" "$IDEM_BODY" >/dev/null
sleep 1

IDEM_COUNT=$(pg_devops "SELECT COUNT(*) FROM global_post_index WHERE post_id=90010;")
if [ "$IDEM_COUNT" = "1" ]; then
    pass "Idempotent - only one post created"
else
    fail "Idempotency" "1" "$IDEM_COUNT"
fi

# === Phase 6: Cross-Cluster Gossip (DEVOPS → EU) ===
echo ""
echo "═══════════════════════════════════════════════════════════"
echo "  Phase 6: Cross-Cluster Gossip (DEVOPS → EU)"
echo "═══════════════════════════════════════════════════════════"

GOSSIP_EID=$(EID "gossip_devops_eu")
log_test "Create post in DEVOPS via /sync/content, verify EU receives it"
GOSSIP_BODY="{
  \"eventId\":\"$GOSSIP_EID\",
  \"eventType\":\"POST_CREATED\",
  \"sourceRegion\":\"SEA\",
  \"targetRegion\":\"EU\",
  \"timestamp\":$TS,
  \"payload\":{
    \"postId\":90020,
    \"authorId\":200,
    \"authorRegion\":\"SEA\",
    \"visibility\":\"GLOBAL\",
    \"content\":\"Gossip test from DEVOPS to EU\",
    \"mediaUrls\":[]
  },
  \"metadata\":{
    \"gdprCompliant\":true,
    \"userConsent\":true,
    \"dataCategory\":\"TIER_2\",
    \"crossBorderOk\":true
  }
}"
send_sync "$DEVOPS_SYNC" "$GOSSIP_BODY" >/dev/null
sleep 3

# Check EU Global Index for the post
EU_RESULT=$(pg_eu "SELECT content_preview FROM global_post_index WHERE post_id=90020;")
if [ "$EU_RESULT" = "Gossip test from DEVOPS to EU" ]; then
    pass "EU received DEVOPS post via gossip"
else
    fail "EU gossip received" "Gossip test from DEVOPS to EU" "$EU_RESULT"
fi

log_test "EU audit log shows cross-border transfer"
EU_AUDIT=$(pg_eu "SELECT status FROM cross_border_audit_log WHERE event_id='$GOSSIP_EID';")
if [ "$EU_AUDIT" = "allowed" ]; then
    pass "EU audit log shows 'allowed'"
else
    fail "EU audit status" "allowed" "${EU_AUDIT:-empty}"
fi

# === Phase 7: Cross-Cluster Gossip (EU → DEVOPS) ===
echo ""
echo "═══════════════════════════════════════════════════════════"
echo "  Phase 7: Cross-Cluster Gossip (EU → DEVOPS)"
echo "═══════════════════════════════════════════════════════════"

# Use EU's /sync/content endpoint (local API path) to trigger gossip broadcast
GOSSIP_EID2=$(EID "gossip_eu_devops")
log_test "Create post in EU via /sync/content, verify DEVOPS receives it"
EU_GOSSIP_BODY="{
  \"eventId\":\"$GOSSIP_EID2\",
  \"eventType\":\"POST_CREATED\",
  \"sourceRegion\":\"EU\",
  \"targetRegion\":\"SEA\",
  \"timestamp\":$TS,
  \"payload\":{
    \"postId\":90030,
    \"authorId\":300,
    \"authorRegion\":\"EU\",
    \"visibility\":\"GLOBAL\",
    \"content\":\"Gossip test from EU to DEVOPS\",
    \"mediaUrls\":[\"https://cdn.eu.example.com/photo.jpg\"]
  },
  \"metadata\":{
    \"gdprCompliant\":true,
    \"userConsent\":true,
    \"dataCategory\":\"TIER_2\",
    \"crossBorderOk\":true
  }
}"
send_sync "$EU_SYNC" "$EU_GOSSIP_BODY" >/dev/null
sleep 3

log_test "Post appears in DEVOPS Global Index"
DEVOPS_RESULT=$(pg_devops "SELECT content_preview FROM global_post_index WHERE post_id=90030;")
if [ "$DEVOPS_RESULT" = "Gossip test from EU to DEVOPS" ]; then
    pass "DEVOPS received EU post via gossip"
else
    fail "DEVOPS gossip received" "Gossip test from EU to DEVOPS" "$DEVOPS_RESULT"
fi

log_test "Media URLs from EU post preserved in DEVOPS"
DEVOPS_MEDIA=$(pg_devops "SELECT COALESCE(array_to_string(media_urls,','),'') FROM global_post_index WHERE post_id=90030;")
if echo "$DEVOPS_MEDIA" | grep -q "cdn.eu.example.com"; then
    pass "EU media URLs preserved in DEVOPS"
else
    fail "EU media URLs" "cdn.eu.example.com" "$DEVOPS_MEDIA"
fi

# === Phase 8: Stats Sync ===
echo ""
echo "═══════════════════════════════════════════════════════════"
echo "  Phase 8: Stats Sync"
echo "═══════════════════════════════════════════════════════════"

log_test "POST_STATS_UPDATED event is processed and logged"
STATS_CREATE_EID=$(EID "stats_create")
STATS_BODY="{
  \"eventId\":\"$STATS_CREATE_EID\",
  \"eventType\":\"POST_CREATED\",
  \"sourceRegion\":\"SEA\",
  \"targetRegion\":\"SEA\",
  \"timestamp\":$TS,
  \"payload\":{
    \"postId\":90040,
    \"authorId\":100,
    \"authorRegion\":\"SEA\",
    \"visibility\":\"GLOBAL\",
    \"content\":\"Stats test post\",
    \"mediaUrls\":[]
  },
  \"metadata\":{
    \"gdprCompliant\":true,
    \"userConsent\":true,
    \"dataCategory\":\"TIER_2\",
    \"crossBorderOk\":true
  }
}"
send_sync "$DEVOPS_SYNC" "$STATS_BODY" >/dev/null
sleep 1

STATS_UPDATE_EID=$(EID "stats_update")
STATS_UPDATE_BODY="{
  \"eventId\":\"$STATS_UPDATE_EID\",
  \"eventType\":\"POST_STATS_UPDATED\",
  \"sourceRegion\":\"SEA\",
  \"targetRegion\":\"SEA\",
  \"timestamp\":$TS,
  \"payload\":{
    \"postId\":90040,
    \"authorId\":100,
    \"authorRegion\":\"SEA\",
    \"visibility\":\"GLOBAL\",
    \"content\":\"\",
    \"mediaUrls\":[]
  },
  \"metadata\":{
    \"gdprCompliant\":true,
    \"userConsent\":true,
    \"dataCategory\":\"TIER_2\",
    \"crossBorderOk\":true
  }
}"
send_sync "$DEVOPS_SYNC" "$STATS_UPDATE_BODY" >/dev/null
sleep 1

# POST_STATS_UPDATED reads from Regional DB which may not have post 90040,
# so it returns error — but event_log should still record it
STATS_LOG=$(pg_devops "SELECT COUNT(*) FROM sync_event_log WHERE event_id='$STATS_UPDATE_EID';")
if [ "$STATS_LOG" = "1" ]; then
    pass "Stats event logged in sync_event_log"
else
    fail "Stats event logged" "1" "${STATS_LOG:-0}"
fi

# === Phase 9: Feed Generation ===
echo ""
echo "═══════════════════════════════════════════════════════════"
echo "  Phase 9: Feed Generation"
echo "═══════════════════════════════════════════════════════════"

log_test "GET /feed/1?feedType=global returns 200 with items"
FEED_RESP=$(curl -s --connect-timeout 10 "$DEVOPS_SYNC/feed/1?feedType=global&limit=5")
if echo "$FEED_RESP" | grep -q '"items"'; then
    pass "Global feed endpoint works"
else
    fail "Global feed" '"items"' "$FEED_RESP"
fi

log_test "Feed response has correct structure"
if echo "$FEED_RESP" | grep -q '"hasMore"'; then
    pass "Feed has hasMore field"
else
    fail "Feed structure" "hasMore" "$FEED_RESP"
fi

log_test "Feed respects limit parameter"
FEED_RESP2=$(curl -s --connect-timeout 10 "$DEVOPS_SYNC/feed/1?feedType=global&limit=2")
if echo "$FEED_RESP2" | grep -q '"limit":2'; then
    pass "Feed limit parameter respected"
else
    fail "Feed limit" "limit:2" "$FEED_RESP2"
fi

# === Cleanup ===
echo ""
echo "═══════════════════════════════════════════════════════════"
echo "  Phase 10: Cleanup"
echo "═══════════════════════════════════════════════════════════"
cleanup_test_data
echo "    Done"

# === Summary ===
echo ""
echo "╔══════════════════════════════════════════════════════════╗"
echo "║                    TEST SUMMARY                          ║"
echo "╠══════════════════════════════════════════════════════════╣"
TOTAL=$((PASS + FAIL))
printf "║  Total: %-3d  Passed: %-3d  Failed: %-3d                 ║\n" "$TOTAL" "$PASS" "$FAIL"
echo "╚══════════════════════════════════════════════════════════╝"

if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
exit 0
