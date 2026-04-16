-- Test migration 002: Add email column to test_users
ALTER TABLE test_users ADD COLUMN IF NOT EXISTS email VARCHAR(255);
