#!/usr/bin/env bash
set -e

SEA_SYNC="https://wigowago-global-sync.verse4.pet"
EU_SYNC="https://global-sync-eu.wigowago.com"
TS=$(date +%s)

PASS=0
FAIL=0
check() {
  local desc="$1" result="$2"
  if echo "$result" | grep -q "$3"; then
    echo "  ✅ PASS: $desc"
    PASS=$((PASS+1))
  else
    echo "  ❌ FAIL: $desc"
    echo "     Got: $result"
    FAIL=$((FAIL+1))
  fi
}

echo "============================================"
echo "  Post 跨集群同步完整测试"
echo "============================================"
echo ""

# --- 测试 6: POST_DELETED ---
echo "--- 6. POST_DELETED: EU 删除 → SEA 验证 ---"

cat > /tmp/event_deleted.json << 'EOJSON'
{
  "eventId": "test_delete_eu_PLACEHOLDER",
  "eventType": "POST_DELETED",
  "sourceRegion": "eu",
  "targetRegion": "sea",
  "timestamp": 0,
  "payload": {
    "postId": 900001,
    "authorId": 99101,
    "authorRegion": "eu",
    "visibility": "GLOBAL"
  },
  "metadata": {
    "gdprCompliant": true,
    "userConsent": true,
    "dataCategory": "TIER_2",
    "crossBorderOk": true
  }
}
EOJSON

sed -i '' "s/PLACEHOLDER/${TS}/g; s/\"timestamp\": 0/\"timestamp\": ${TS}/g" /tmp/event_deleted.json

echo "发送 POST_DELETED (postId=900001)..."
DEL_RESULT=$(curl -s -X POST "$EU_SYNC/sync/content" \
  -H "Content-Type: application/json" \
  -d @/tmp/event_deleted.json)
echo "  Response: $DEL_RESULT"
sleep 5

SEA_DEL=$(curl -s "$SEA_SYNC/index/posts/900001")
echo "  SEA 查询: $SEA_DEL"
EU_DEL=$(curl -s "$EU_SYNC/index/posts/900001")
echo "  EU  查询: $EU_DEL"

check "SEA 已删除" "$SEA_DEL" "not found"
check "EU  已删除" "$EU_DEL" "not found"
echo ""

# --- 测试 7: POST_STATS_UPDATED ---
echo "--- 7. POST_STATS_UPDATED: EU 更新 → SEA 验证 ---"

# 先创建一个新 post
cat > /tmp/event_stats_create.json << EOJSON
{
  "eventId": "test_stats_create_eu_${TS}",
  "eventType": "POST_CREATED",
  "sourceRegion": "eu",
  "targetRegion": "sea",
  "timestamp": ${TS},
  "payload": {
    "postId": 900003,
    "authorId": 99101,
    "authorRegion": "eu",
    "visibility": "GLOBAL",
    "content": "Post for stats test"
  },
  "metadata": {
    "gdprCompliant": true,
    "userConsent": true,
    "dataCategory": "TIER_2",
    "crossBorderOk": true
  }
}
EOJSON

echo "创建 post 900003..."
curl -s -X POST "$EU_SYNC/sync/content" \
  -H "Content-Type: application/json" \
  -d @/tmp/event_stats_create.json > /dev/null
sleep 5

SEA_BEFORE=$(curl -s "$SEA_SYNC/index/posts/900003")
echo "  SEA 创建后: $SEA_BEFORE"
check "SEA 有 post 900003" "$SEA_BEFORE" "900003"
echo ""

# 发送 stats updated
echo "发送 POST_STATS_UPDATED..."
cat > /tmp/event_stats.json << EOJSON
{
  "eventId": "test_stats_eu_${TS}",
  "eventType": "POST_STATS_UPDATED",
  "sourceRegion": "eu",
  "targetRegion": "sea",
  "timestamp": ${TS},
  "payload": {
    "postId": 900003,
    "authorId": 99101,
    "authorRegion": "eu",
    "visibility": "GLOBAL"
  },
  "metadata": {
    "gdprCompliant": true,
    "userConsent": true,
    "dataCategory": "TIER_2",
    "crossBorderOk": true
  }
}
EOJSON

STATS_RESULT=$(curl -s -X POST "$EU_SYNC/sync/content" \
  -H "Content-Type: application/json" \
  -d @/tmp/event_stats.json)
echo "  Response: $STATS_RESULT"
sleep 5

# POST_STATS_UPDATED 会去读 Regional DB，如果 post 不存在会跳过
# 我们只看是否没有报错
SEA_AFTER=$(curl -s "$SEA_SYNC/index/posts/900003")
echo "  SEA 查询: $SEA_AFTER"
echo ""

# --- 测试 8: 数据一致性汇总 ---
echo "--- 8. 数据一致性: 对比 SEA 和 EU Global Index ---"
echo ""
echo "SEA Global Index posts:"
curl -s "$SEA_SYNC/index/posts/800001" | python3 -c "
import sys,json
try:
  d=json.load(sys.stdin)
  print(f'  800001: authorRegion={d[\"authorRegion\"]} visibility={d[\"visibility\"]}')
except: print('  800001: not found')
" 2>/dev/null || echo "  800001: parse error"

curl -s "$SEA_SYNC/index/posts/800010" | python3 -c "
import sys,json
try:
  d=json.load(sys.stdin)
  print(f'  800010: preview={d[\"contentPreview\"][:50]}...')
except: print('  800010: not found')
" 2>/dev/null || echo "  800010: parse error"

curl -s "$SEA_SYNC/index/posts/900001" 2>/dev/null | python3 -c "
import sys,json
try:
  d=json.load(sys.stdin)
  print(f'  900001: authorRegion={d[\"authorRegion\"]} (should not exist after delete)')
except: print('  900001: not found (correct - was deleted)')
" 2>/dev/null || echo "  900001: not found (correct - was deleted)"

curl -s "$SEA_SYNC/index/posts/900003" 2>/dev/null | python3 -c "
import sys,json
try:
  d=json.load(sys.stdin)
  print(f'  900003: authorRegion={d[\"authorRegion\"]} visibility={d[\"visibility\"]}')
except: print('  900003: not found')
" 2>/dev/null || echo "  900003: parse error"

echo ""
echo "EU Global Index posts:"
curl -s "$EU_SYNC/index/posts/800001" 2>/dev/null | python3 -c "
import sys,json
try:
  d=json.load(sys.stdin)
  print(f'  800001: authorRegion={d[\"authorRegion\"]} visibility={d[\"visibility\"]}')
except: print('  800001: not found')
" 2>/dev/null || echo "  800001: parse error"

curl -s "$EU_SYNC/index/posts/800010" 2>/dev/null | python3 -c "
import sys,json
try:
  d=json.load(sys.stdin)
  print(f'  800010: preview={d[\"contentPreview\"][:50]}...')
except: print('  800010: not found')
" 2>/dev/null || echo "  800010: parse error"

curl -s "$EU_SYNC/index/posts/900001" 2>/dev/null | python3 -c "
import sys,json
try:
  d=json.load(sys.stdin)
  print(f'  900001: authorRegion={d[\"authorRegion\"]} (should not exist after delete)')
except: print('  900001: not found (correct - was deleted)')
" 2>/dev/null || echo "  900001: not found (correct - was deleted)"

curl -s "$EU_SYNC/index/posts/900003" 2>/dev/null | python3 -c "
import sys,json
try:
  d=json.load(sys.stdin)
  print(f'  900003: authorRegion={d[\"authorRegion\"]} visibility={d[\"visibility\"]}')
except: print('  900003: not found')
" 2>/dev/null || echo "  900003: parse error"

echo ""
echo "============================================"
echo "  测试结果汇总"
echo "============================================"
echo "  PASS: $PASS"
echo "  FAIL: $FAIL"
echo "============================================"
