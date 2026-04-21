-- Migration 003: Cross-Border Audit Log (GDPR compliance tracking)
CREATE TABLE IF NOT EXISTS cross_border_audit_log (
    log_id BIGSERIAL PRIMARY KEY,
    event_id VARCHAR(64) NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    data_subject_id BIGINT NOT NULL,
    source_region VARCHAR(16) NOT NULL,
    target_region VARCHAR(16) NOT NULL,
    data_type VARCHAR(50) NOT NULL,
    legal_basis VARCHAR(100),
    user_consent BOOLEAN DEFAULT false,
    status VARCHAR(20) NOT NULL,
    metadata JSONB
);

CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON cross_border_audit_log(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_subject ON cross_border_audit_log(data_subject_id);
CREATE INDEX IF NOT EXISTS idx_audit_status ON cross_border_audit_log(status);

-- Ensure region columns are wide enough for 3+ char codes (sea, usw2, etc.)
ALTER TABLE cross_border_audit_log ALTER COLUMN source_region TYPE VARCHAR(16);
ALTER TABLE cross_border_audit_log ALTER COLUMN target_region TYPE VARCHAR(16);
