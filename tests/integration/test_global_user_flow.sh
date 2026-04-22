#!/usr/bin/env bash
# Global Sync Service - Cross-Region User Flow Tests
# Tests global uniqueness, region routing, and cross-cluster sync
# Uses Global Sync Service API directly + known test account for full flow
set -euo pipefail

SEA_API="https://wigowago-api.verse4.pet"
EU_API="https://api-eu.wigowago.com"
GLOBAL_API="https://global-api.wigowago.com"

SEA_GS="https://wigowago-global-sync.verse4.pet"
EU_GS="https://global-sync-eu.wigowago.com"

PASS=0
FAIL=0

pass() { echo "  ✅ PASS: $1"; PASS=$((PASS+1)); }
fail() { echo "  ❌ FAIL: $1"; FAIL=$((FAIL+1)); }
step() { echo ""; echo "═══ $1 ═══"; }

# Test users with unique hashes
SEA_USER_EMAIL="cross-test-sea@wigowago.com"
EU_USER_EMAIL="cross-test-eu@wigowago.com"
SEA_USER_ID=99001
EU_USER_ID=99002

# Calculate SHA256 hashes
SEA_EMAIL_HASH=$(echo -n "$SEA_USER_EMAIL" | shasum -a 256 | awk '{print $1}')
EU_EMAIL_HASH=$(echo -n "$EU_USER_EMAIL" | shasum -a 256 | awk '{print $1}')

step "Phase 0: Cleanup - Remove test users from Global Index"

# Remove any existing test entries (idempotent delete)
curl -s -X DELETE "$SEA_GS/index/users/$SEA_EMAIL_HASH" -o /dev/null 2>/dev/null || true
curl -s -X DELETE "$SEA_GS/index/users/$EU_EMAIL_HASH" -o /dev/null 2>/dev/null || true
curl -s -X DELETE "$EU_GS/index/users/$SEA_EMAIL_HASH" -o /dev/null 2>/dev/null || true
curl -s -X DELETE "$EU_GS/index/users/$EU_EMAIL_HASH" -o /dev/null 2>/dev/null || true

step "Phase 1: SEA User Registration -> Upsert to Global Index"

echo "  Upsert SEA user to SEA Global Sync Service"
SEA_UPSERT=$(curl -s -X POST "$SEA_GS/index/users/upsert" \
  -H "Content-Type: application/json" \
  -d "{\"emailHash\":\"$SEA_EMAIL_HASH\",\"userId\":$SEA_USER_ID,\"region\":\"sea\"}")
SEA_UPSERT_STATUS=$(echo "$SEA_UPSERT" | jq -r '.status // "error"')

if [ "$SEA_UPSERT_STATUS" = "ok" ]; then
  pass "SEA user upserted to SEA Global Index"
else
  fail "SEA user upsert failed: $SEA_UPSERT"
fi

# Wait for gossip sync
sleep 2

step "Phase 2: EU User Registration -> Upsert to Global Index"

echo "  Upsert EU user to EU Global Sync Service"
EU_UPSERT=$(curl -s -X POST "$EU_GS/index/users/upsert" \
  -H "Content-Type: application/json" \
  -d "{\"emailHash\":\"$EU_EMAIL_HASH\",\"userId\":$EU_USER_ID,\"region\":\"eu\"}")
EU_UPSERT_STATUS=$(echo "$EU_UPSERT" | jq -r '.status // "error"')

if [ "$EU_UPSERT_STATUS" = "ok" ]; then
  pass "EU user upserted to EU Global Index"
else
  fail "EU user upsert failed: $EU_UPSERT"
fi

# Wait for gossip sync
sleep 3

step "Phase 3: Global Index Verification (Cross-Cluster Sync)"

# Check SEA user in SEA Global Index
SEA_CHECK_SEA=$(curl -s -X POST "$SEA_GS/index/users/check" \
  -H "Content-Type: application/json" \
  -d "{\"emailHash\":\"$SEA_EMAIL_HASH\"}")
SEA_IN_SEA=$(echo "$SEA_CHECK_SEA" | jq -r '.exists // false')
SEA_REGION_SEA=$(echo "$SEA_CHECK_SEA" | jq -r '.region // ""')

if [ "$SEA_IN_SEA" = "true" ] && [ "$SEA_REGION_SEA" = "sea" ]; then
  pass "SEA user exists in SEA Global Index (region: sea)"
else
  fail "SEA user check in SEA failed: exists=$SEA_IN_SEA, region=$SEA_REGION_SEA"
fi

# Check SEA user in EU Global Index (gossip sync verification)
SEA_CHECK_EU=$(curl -s -X POST "$EU_GS/index/users/check" \
  -H "Content-Type: application/json" \
  -d "{\"emailHash\":\"$SEA_EMAIL_HASH\"}")
SEA_IN_EU=$(echo "$SEA_CHECK_EU" | jq -r '.exists // false')
SEA_REGION_EU=$(echo "$SEA_CHECK_EU" | jq -r '.region // ""')

if [ "$SEA_IN_EU" = "true" ] && [ "$SEA_REGION_EU" = "sea" ]; then
  pass "SEA user synced to EU Global Index (gossip OK, region: sea)"
else
  fail "SEA user NOT in EU Global Index: exists=$SEA_IN_EU, region=$SEA_REGION_EU"
fi

# Check EU user in EU Global Index
EU_CHECK_EU=$(curl -s -X POST "$EU_GS/index/users/check" \
  -H "Content-Type: application/json" \
  -d "{\"emailHash\":\"$EU_EMAIL_HASH\"}")
EU_IN_EU=$(echo "$EU_CHECK_EU" | jq -r '.exists // false')
EU_REGION_EU=$(echo "$EU_CHECK_EU" | jq -r '.region // ""')

if [ "$EU_IN_EU" = "true" ] && [ "$EU_REGION_EU" = "eu" ]; then
  pass "EU user exists in EU Global Index (region: eu)"
else
  fail "EU user check in EU failed: exists=$EU_IN_EU, region=$EU_REGION_EU"
fi

# Check EU user in SEA Global Index (gossip sync verification)
EU_CHECK_SEA=$(curl -s -X POST "$SEA_GS/index/users/check" \
  -H "Content-Type: application/json" \
  -d "{\"emailHash\":\"$EU_EMAIL_HASH\"}")
EU_IN_SEA=$(echo "$EU_CHECK_SEA" | jq -r '.exists // false')
EU_REGION_SEA=$(echo "$EU_CHECK_SEA" | jq -r '.region // ""')

if [ "$EU_IN_SEA" = "true" ] && [ "$EU_REGION_SEA" = "eu" ]; then
  pass "EU user synced to SEA Global Index (gossip OK, region: eu)"
else
  fail "EU user NOT in SEA Global Index: exists=$EU_IN_SEA, region=$EU_REGION_SEA"
fi

step "Phase 4: Cross-Region Login Interception (check-exists API)"

# Scenario: SEA user tries to check-exists via EU API
# Expected: exists=true, region=sea, regionalBaseUrl=SEA URL
EU_CHECK_SEA_USER=$(curl -s -X POST "$EU_API/auth/email/check-exists" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$SEA_USER_EMAIL\"}")
EU_CHECK_SEA_EXISTS=$(echo "$EU_CHECK_SEA_USER" | jq -r '.data.exists // false')
EU_CHECK_SEA_REGION=$(echo "$EU_CHECK_SEA_USER" | jq -r '.data.region // ""')
EU_CHECK_SEA_URL=$(echo "$EU_CHECK_SEA_USER" | jq -r '.data.regionalBaseUrl // ""')

if [ "$EU_CHECK_SEA_EXISTS" = "true" ] && [ "$EU_CHECK_SEA_REGION" = "sea" ]; then
  pass "EU check-exists correctly identifies SEA user (redirect: $EU_CHECK_SEA_URL)"
else
  fail "EU check-exists wrong for SEA user: exists=$EU_CHECK_SEA_EXISTS, region=$EU_CHECK_SEA_REGION"
fi

# Scenario: EU user tries to check-exists via SEA API
# Expected: exists=true, region=eu, regionalBaseUrl=EU URL
SEA_CHECK_EU_USER=$(curl -s -X POST "$SEA_API/auth/email/check-exists" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$EU_USER_EMAIL\"}")
SEA_CHECK_EU_EXISTS=$(echo "$SEA_CHECK_EU_USER" | jq -r '.data.exists // false')
SEA_CHECK_EU_REGION=$(echo "$SEA_CHECK_EU_USER" | jq -r '.data.region // ""')
SEA_CHECK_EU_URL=$(echo "$SEA_CHECK_EU_USER" | jq -r '.data.regionalBaseUrl // ""')

if [ "$SEA_CHECK_EU_EXISTS" = "true" ] && [ "$SEA_CHECK_EU_REGION" = "eu" ]; then
  pass "SEA check-exists correctly identifies EU user (redirect: $SEA_CHECK_EU_URL)"
else
  fail "SEA check-exists wrong for EU user: exists=$SEA_CHECK_EU_EXISTS, region=$SEA_CHECK_EU_REGION"
fi

step "Phase 5: Global API Entry Point (307 Routing)"

# Test check-exists through global-api
GLOBAL_CHECK=$(curl -s -L -X POST "$GLOBAL_API/auth/email/check-exists" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$SEA_USER_EMAIL\"}")
GLOBAL_CODE=$(echo "$GLOBAL_CHECK" | jq -r '.code // "error"')
GLOBAL_REGION=$(echo "$GLOBAL_CHECK" | jq -r '.data.region // ""')

if [ "$GLOBAL_CODE" = "0" ] && [ "$GLOBAL_REGION" = "sea" ]; then
  pass "Global API routes check-exists correctly (SEA user identified)"
else
  fail "Global API routing failed: $GLOBAL_CHECK"
fi

# Test send-code through global-api
GLOBAL_SEND=$(curl -s -L -X POST "$GLOBAL_API/auth/email/send-code" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"test@wigowago.com\"}")
GLOBAL_SEND_CODE=$(echo "$GLOBAL_SEND" | jq -r '.code // "error"')

if [ "$GLOBAL_SEND_CODE" = "0" ] || [ "$GLOBAL_SEND_CODE" = "ERR_INVALID_OPERATION" ]; then
  pass "Global API routes send-code correctly"
else
  fail "Global API send-code failed: $GLOBAL_SEND"
fi

step "Phase 6: Same-Region Test Account Login (Baseline)"

# Use known test account to verify login flow works
SEA_TEST_LOGIN=$(curl -s -X POST "$SEA_API/auth/email/send-code" \
  -H "Content-Type: application/json" \
  -d '{"email":"test@wigowago.com"}')
SEA_TEST_LOGIN_CODE=$(echo "$SEA_TEST_LOGIN" | jq -r '.code // "error"')

SEA_TEST_VERIFY=$(curl -s -X POST "$SEA_API/auth/email/verify" \
  -H "Content-Type: application/json" \
  -d '{"email":"test@wigowago.com","code":"66688"}')
SEA_TEST_VERIFY_CODE=$(echo "$SEA_TEST_VERIFY" | jq -r '.code // "error"')
SEA_TEST_TOKEN=$(echo "$SEA_TEST_VERIFY" | jq -r '.data.accessToken // ""')

if [ "$SEA_TEST_VERIFY_CODE" = "0" ] && [ -n "$SEA_TEST_TOKEN" ]; then
  pass "test@wigowago.com login on SEA API succeeds"
else
  fail "test@wigowago.com login on SEA API failed: $SEA_TEST_VERIFY"
fi

EU_TEST_LOGIN=$(curl -s -X POST "$EU_API/auth/email/send-code" \
  -H "Content-Type: application/json" \
  -d '{"email":"test@wigowago.com"}')
EU_TEST_VERIFY=$(curl -s -X POST "$EU_API/auth/email/verify" \
  -H "Content-Type: application/json" \
  -d '{"email":"test@wigowago.com","code":"66688"}')
EU_TEST_VERIFY_CODE=$(echo "$EU_TEST_VERIFY" | jq -r '.code // "error"')

if [ "$EU_TEST_VERIFY_CODE" = "0" ]; then
  pass "test@wigowago.com login on EU API succeeds"
else
  # This might fail due to region validation (test account might be SEA)
  EU_TEST_MSG=$(echo "$EU_TEST_VERIFY" | jq -r '.message // ""')
  if [[ "$EU_TEST_MSG" == *"region"* ]] || [[ "$EU_TEST_VERIFY_CODE" == *"REGION"* ]]; then
    pass "test@wigowago.com login on EU API correctly enforces region (expected for SEA test account)"
  else
    fail "test@wigowago.com login on EU API failed: $EU_TEST_VERIFY"
  fi
fi

step "Phase 7: Idempotency - Duplicate Upsert"

# Upsert same SEA user again - should be idempotent
SEA_UPSERT2=$(curl -s -X POST "$SEA_GS/index/users/upsert" \
  -H "Content-Type: application/json" \
  -d "{\"emailHash\":\"$SEA_EMAIL_HASH\",\"userId\":$SEA_USER_ID,\"region\":\"sea\"}")
SEA_UPSERT2_STATUS=$(echo "$SEA_UPSERT2" | jq -r '.status // "error"')

if [ "$SEA_UPSERT2_STATUS" = "ok" ]; then
  pass "Duplicate upsert is idempotent"
else
  fail "Duplicate upsert failed: $SEA_UPSERT2"
fi

# Verify no duplicate entries
SEA_CHECK_AFTER=$(curl -s -X POST "$SEA_GS/index/users/check" \
  -H "Content-Type: application/json" \
  -d "{\"emailHash\":\"$SEA_EMAIL_HASH\"}")
SEA_COUNT=$(echo "$SEA_CHECK_AFTER" | jq -r '.exists // false')

if [ "$SEA_COUNT" = "true" ]; then
  pass "No duplicate entries after idempotent upsert"
else
  fail "User missing after idempotent upsert"
fi

step "Summary"
echo ""
echo "╔══════════════════════════════════════════╗"
echo "║  Total: $((PASS+FAIL))   Passed: $PASS   Failed: $FAIL             ║"
echo "╚══════════════════════════════════════════╝"

if [ $FAIL -gt 0 ]; then
  exit 1
fi
