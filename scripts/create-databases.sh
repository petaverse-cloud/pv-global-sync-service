#!/usr/bin/env bash
# Create required databases on shared Azure RDS PostgreSQL
# Usage: ./scripts/create-databases.sh
#
# Prerequisites:
#   - psql installed
#   - PGPASSWORD env var or .pgpass configured
#
# Shared RDS: rds-postgres-dev.postgres.database.azure.com:5432
# User: postgres

set -euo pipefail

DB_HOST="${1:-rds-postgres-dev.postgres.database.azure.com}"
DB_USER="${2:-postgres}"

echo "Creating databases on ${DB_HOST}..."

# Check databases exist
for dbname in wigowago_regional wigowago_global_index; do
  echo "Creating database: ${dbname}"
  PGPASSWORD="${PGPASSWORD}" psql -h "${DB_HOST}" -U "${DB_USER}" -d postgres -c \
    "SELECT 'Database ${dbname} already exists' WHERE EXISTS (SELECT 1 FROM pg_database WHERE datname='${dbname}');" 2>/dev/null || true

  PGPASSWORD="${PGPASSWORD}" psql -h "${DB_HOST}" -U "${DB_USER}" -d postgres -tc \
    "SELECT 1 FROM pg_database WHERE datname='${dbname}'" | grep -q 1 || \
    PGPASSWORD="${PGPASSWORD}" psql -h "${DB_HOST}" -U "${DB_USER}" -d postgres -c \
    "CREATE DATABASE ${dbname};"

  echo "  -> ${dbname}: done"
done

echo ""
echo "Databases created. Run schema initialization:"
echo "  psql -h ${DB_HOST} -U ${DB_USER} -d wigowago_regional -f scripts/schema.sql"
echo "  psql -h ${DB_HOST} -U ${DB_USER} -d wigowago_global_index -f scripts/schema.sql"
