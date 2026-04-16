// Package migrations embeds all SQL migration files for the Global Sync Service.
//
// Two embedded filesystems are provided:
//   - RegionalFS: migrations for the Regional DB (users, user_follows, etc.)
//   - GlobalIndexFS: migrations for the Global Index DB (global_post_index, etc.)
//
// These are run automatically on server startup via pkg/migrate.Runner.
package migrations

import "embed"

// RegionalFS contains migrations for the Regional DB.
// These create the minimal tables needed by FeedGenerator (users, user_follows).
// Full wigowago user schema is managed by wigowago-api TypeORM migrations.
//
//go:embed regional/*.sql
var RegionalFS embed.FS

// GlobalIndexFS contains migrations for the Global Index DB.
// These create the tables needed for cross-region post synchronization.
//
//go:embed global_index/*.sql
var GlobalIndexFS embed.FS
