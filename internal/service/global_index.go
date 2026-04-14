// Package service contains business logic for the Global Sync Service.
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/model"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

// GlobalIndexPost is a simplified post representation from the global index.
type GlobalIndexPost struct {
	PostID         int64
	AuthorID       int64
	ContentPreview string
	LikesCount     int
	CommentsCount  int
	SharesCount    int
	ViewsCount     int
	CreatedAt      time.Time
}

// GlobalIndexService manages operations on the global_post_index table.
type GlobalIndexService struct {
	db  *pgxpool.Pool
	log *logger.Logger
}

// NewGlobalIndexService creates a new service instance.
func NewGlobalIndexService(db *pgxpool.Pool, log *logger.Logger) *GlobalIndexService {
	return &GlobalIndexService{db: db, log: log}
}

// InsertPost inserts a new post into the global index.
func (s *GlobalIndexService) InsertPost(ctx context.Context, event *model.CrossRegionSyncEvent) error {
	now := time.Now().UTC()

	query := `
		INSERT INTO global_post_index (
			post_id, author_id, author_region, content_preview, visibility,
			hashtags, mentions, likes_count, comments_count, shares_count, views_count,
			gdpr_compliant, user_consent, data_category, created_at, synced_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, 0, 0, 0, 0,
			$8, $9, $10, $11, $12
		)
		ON CONFLICT (post_id) DO NOTHING
	`

	hashtags := extractHashtags(event.Payload.Content)
	_, err := s.db.Exec(ctx, query,
		event.Payload.PostID,
		event.Payload.AuthorID,
		event.Payload.AuthorRegion,
		truncatePreview(event.Payload.Content, 500),
		event.Payload.Visibility,
		hashtags,
		[]int64{}, // Mentions populated separately when available
		event.Metadata.GDPRCompliant,
		event.Metadata.UserConsent,
		event.Metadata.DataCategory,
		now,
		now,
	)

	if err != nil {
		return fmt.Errorf("insert post %d: %w", event.Payload.PostID, err)
	}

	s.log.Info("Post inserted into global index",
		logger.Int64("post_id", event.Payload.PostID),
		logger.String("region", string(event.Payload.AuthorRegion)),
		logger.String("visibility", string(event.Payload.Visibility)),
	)

	return nil
}

// UpdatePost updates an existing post in the global index.
func (s *GlobalIndexService) UpdatePost(ctx context.Context, event *model.CrossRegionSyncEvent) error {
	query := `
		UPDATE global_post_index
		SET content_preview = $1,
			visibility = $2,
			hashtags = $3,
			updated_at = NOW(),
			synced_at = NOW()
		WHERE post_id = $4
	`

	hashtags := extractHashtags(event.Payload.Content)
	result, err := s.db.Exec(ctx, query,
		truncatePreview(event.Payload.Content, 500),
		event.Payload.Visibility,
		hashtags,
		event.Payload.PostID,
	)
	if err != nil {
		return fmt.Errorf("update post %d: %w", event.Payload.PostID, err)
	}

	rows := result.RowsAffected()
	if rows == 0 {
		s.log.Warn("Post not found in global index for update, inserting instead",
			logger.Int64("post_id", event.Payload.PostID))
		return s.InsertPost(ctx, event)
	}

	s.log.Info("Post updated in global index",
		logger.Int64("post_id", event.Payload.PostID))

	return nil
}

// DeletePost removes a post from the global index (GDPR deletion).
func (s *GlobalIndexService) DeletePost(ctx context.Context, event *model.CrossRegionSyncEvent) error {
	query := `DELETE FROM global_post_index WHERE post_id = $1`

	result, err := s.db.Exec(ctx, query, event.Payload.PostID)
	if err != nil {
		return fmt.Errorf("delete post %d: %w", event.Payload.PostID, err)
	}

	rows := result.RowsAffected()
	s.log.Info("Post deleted from global index",
		logger.Int64("post_id", event.Payload.PostID),
		logger.Int64("rows_affected", rows))

	return nil
}

// UpdateStats updates engagement counts for a post.
func (s *GlobalIndexService) UpdateStats(ctx context.Context, postID int64, likes, comments, shares, views int) error {
	query := `
		UPDATE global_post_index
		SET likes_count = $1,
			comments_count = $2,
			shares_count = $3,
			views_count = $4,
			updated_at = NOW()
		WHERE post_id = $5
	`

	_, err := s.db.Exec(ctx, query, likes, comments, shares, views, postID)
	return err
}

// GetPost retrieves a post from the global index.
func (s *GlobalIndexService) GetPost(ctx context.Context, postID int64) (*model.GlobalPostIndex, error) {
	query := `
		SELECT post_id, author_id, author_region, content_preview, visibility,
		       hashtags, mentions, likes_count, comments_count, shares_count, views_count,
		       gdpr_compliant, user_consent, data_category, created_at, synced_at
		FROM global_post_index
		WHERE post_id = $1
	`

	var post model.GlobalPostIndex
	var createdAt, syncedAt time.Time
	var hashtags, mentions pgtypeArray

	err := s.db.QueryRow(ctx, query, postID).Scan(
		&post.PostID, &post.AuthorID, &post.AuthorRegion,
		&post.ContentPreview, &post.Visibility,
		&hashtags, &mentions,
		&post.LikesCount, &post.CommentsCount, &post.SharesCount, &post.ViewsCount,
		&post.GDPRCompliant, &post.UserConsent, &post.DataCategory,
		&createdAt, &syncedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get post %d: %w", postID, err)
	}

	post.CreatedAt = createdAt
	post.SyncedAt = syncedAt

	return &post, nil
}

// GetPostsByAuthor retrieves all global posts by a specific author.
func (s *GlobalIndexService) GetPostsByAuthor(ctx context.Context, authorID int64, limit int) ([]model.GlobalPostIndex, error) {
	query := `
		SELECT post_id, author_id, author_region, content_preview, visibility,
		       hashtags, likes_count, comments_count, shares_count, views_count,
		       gdpr_compliant, user_consent, data_category, created_at, synced_at
		FROM global_post_index
		WHERE author_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := s.db.Query(ctx, query, authorID, limit)
	if err != nil {
		return nil, fmt.Errorf("query posts by author %d: %w", authorID, err)
	}
	defer rows.Close()

	var posts []model.GlobalPostIndex
	for rows.Next() {
		var p model.GlobalPostIndex
		var createdAt, syncedAt time.Time
		var hashtags pgtypeArray
		if err := rows.Scan(
			&p.PostID, &p.AuthorID, &p.AuthorRegion,
			&p.ContentPreview, &p.Visibility,
			&hashtags,
			&p.LikesCount, &p.CommentsCount, &p.SharesCount, &p.ViewsCount,
			&p.GDPRCompliant, &p.UserConsent, &p.DataCategory,
			&createdAt, &syncedAt,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		p.CreatedAt = createdAt
		p.SyncedAt = syncedAt
		posts = append(posts, p)
	}

	return posts, rows.Err()
}

// pgtypeArray is a helper for scanning PostgreSQL TEXT[] columns.
type pgtypeArray []string

func (a *pgtypeArray) Scan(value interface{}) error {
	if value == nil {
		*a = nil
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("expected []byte, got %T", value)
	}

	// Parse PostgreSQL array format: {tag1,tag2,tag3}
	s := string(bytes)
	if len(s) < 2 || s[0] != '{' || s[len(s)-1] != '}' {
		*a = []string{s}
		return nil
	}

	inner := s[1 : len(s)-1]
	if inner == "" {
		*a = []string{}
		return nil
	}

	// Simple split - handles basic cases without quoted commas
	parts := []string{}
	current := ""
	escaped := false
	for _, ch := range inner {
		if ch == ',' && !escaped {
			parts = append(parts, current)
			current = ""
			continue
		}
		if ch == '"' {
			escaped = !escaped
			continue
		}
		current += string(ch)
	}
	if current != "" {
		parts = append(parts, current)
	}

	*a = parts
	return nil
}

// extractHashtags extracts #hashtags from content text.
func extractHashtags(content string) []string {
	seen := make(map[string]bool)
	var tags []string

	i := 0
	for i < len(content) {
		if content[i] == '#' && i+1 < len(content) {
			j := i + 1
			for j < len(content) && isTagChar(content[j]) {
				j++
			}
			if j > i+1 {
				tag := content[i+1 : j]
				if !seen[tag] {
					seen[tag] = true
					tags = append(tags, tag)
				}
			}
			i = j
		} else {
			i++
		}
	}

	return tags
}

func isTagChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') || b == '_'
}

func truncatePreview(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "..."
}

// GetPostsFromAuthors retrieves global posts from a list of authors.
// Used for the "following" feed pull mode.
func (s *GlobalIndexService) GetPostsFromAuthors(ctx context.Context, authorIDs []int64, limit int) ([]GlobalIndexPost, error) {
	query := `
		SELECT post_id, author_id, content_preview,
		       likes_count, comments_count, shares_count, views_count,
		       created_at
		FROM global_post_index
		WHERE author_id = ANY($1)
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := s.db.Query(ctx, query, authorIDs, limit)
	if err != nil {
		return nil, fmt.Errorf("query posts from authors: %w", err)
	}
	defer rows.Close()

	posts := make([]GlobalIndexPost, 0)
	for rows.Next() {
		var p GlobalIndexPost
		if err := rows.Scan(&p.PostID, &p.AuthorID, &p.ContentPreview,
			&p.LikesCount, &p.CommentsCount, &p.SharesCount, &p.ViewsCount,
			&p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan post: %w", err)
		}
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

// GetGlobalPosts retrieves recent public posts from all authors.
// Used for the "global" feed.
func (s *GlobalIndexService) GetGlobalPosts(ctx context.Context, limit int) ([]GlobalIndexPost, error) {
	query := `
		SELECT post_id, author_id, content_preview,
		       likes_count, comments_count, shares_count, views_count,
		       created_at
		FROM global_post_index
		ORDER BY created_at DESC
		LIMIT $1
	`

	rows, err := s.db.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query global posts: %w", err)
	}
	defer rows.Close()

	posts := make([]GlobalIndexPost, 0)
	for rows.Next() {
		var p GlobalIndexPost
		if err := rows.Scan(&p.PostID, &p.AuthorID, &p.ContentPreview,
			&p.LikesCount, &p.CommentsCount, &p.SharesCount, &p.ViewsCount,
			&p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan post: %w", err)
		}
		posts = append(posts, p)
	}
	return posts, rows.Err()
}

// GetTrendingPosts retrieves posts with highest engagement in the last 24 hours.
// Used for the "trending" feed.
func (s *GlobalIndexService) GetTrendingPosts(ctx context.Context, limit int) ([]GlobalIndexPost, error) {
	query := `
		SELECT post_id, author_id, content_preview,
		       likes_count, comments_count, shares_count, views_count,
		       created_at
		FROM global_post_index
		WHERE created_at > NOW() - INTERVAL '24 hours'
		ORDER BY (likes_count + comments_count*2 + shares_count*3) DESC
		LIMIT $1
	`

	rows, err := s.db.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("query trending posts: %w", err)
	}
	defer rows.Close()

	posts := make([]GlobalIndexPost, 0)
	for rows.Next() {
		var p GlobalIndexPost
		if err := rows.Scan(&p.PostID, &p.AuthorID, &p.ContentPreview,
			&p.LikesCount, &p.CommentsCount, &p.SharesCount, &p.ViewsCount,
			&p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan post: %w", err)
		}
		posts = append(posts, p)
	}
	return posts, rows.Err()
}
