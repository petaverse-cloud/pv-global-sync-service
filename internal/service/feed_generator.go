// Package service contains business logic for the Global Sync Service.
package service

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/postgres"
	redispkg "github.com/petaverse-cloud/pv-global-sync-service/pkg/redis"
)

// FeedGenerator implements the Push/Pull hybrid feed generation strategy.
//
// Push (Fan-out): For users with < pushThreshold followers, pre-compute
// feed items into Redis ZSETs when a new post is created.
//
// Pull (Fan-in): For users with >= pushThreshold followers, skip
// pre-computation. Feed items are aggregated at read time from Global Index.
//
// TTL:
//   - Following feed: 5 minutes
//   - Global feed: 15 minutes
//   - Trending feed: 1 minute
type FeedGenerator struct {
	regionalDB    *postgres.Manager
	redis         *redispkg.Client
	indexSvc      *GlobalIndexService
	log           *logger.Logger
	pushThreshold int
	feedTTLs      map[string]time.Duration
}

// FeedTTLs returns default cache TTLs for each feed type.
func FeedTTLs() map[string]time.Duration {
	return map[string]time.Duration{
		"following": 5 * time.Minute,
		"global":    15 * time.Minute,
		"trending":  1 * time.Minute,
	}
}

// NewFeedGenerator creates a new feed generator.
func NewFeedGenerator(db *postgres.Manager, redis *redispkg.Client, indexSvc *GlobalIndexService, log *logger.Logger, pushThreshold int) *FeedGenerator {
	return &FeedGenerator{
		regionalDB:    db,
		redis:         redis,
		indexSvc:      indexSvc,
		log:           log,
		pushThreshold: pushThreshold,
		feedTTLs:      FeedTTLs(),
	}
}

// HandleNewPost triggers feed generation when a new post enters the global index.
// Decides Push vs Pull based on author's follower count.
func (f *FeedGenerator) HandleNewPost(ctx context.Context, authorID int64, postID int64) error {
	followerCount, err := f.getFollowerCount(ctx, authorID)
	if err != nil {
		f.log.Error("Failed to get follower count for feed generation",
			logger.Int64("author_id", authorID),
			logger.Error(err))
		// Fall back to pull mode (safe default)
		return nil
	}

	if followerCount < f.pushThreshold {
		return f.pushMode(ctx, authorID, postID, followerCount)
	}

	f.log.Info("Using pull mode for celebrity post",
		logger.Int64("author_id", authorID),
		logger.Int64("post_id", postID),
		logger.Int("followers", followerCount))
	return nil
}

// HandleDeletedPost removes the post from all feed caches.
func (f *FeedGenerator) HandleDeletedPost(ctx context.Context, postID int64) error {
	// Invalidate all feed types containing this post
	// Since Redis ZSET doesn't support reverse lookup efficiently,
	// we rely on TTL expiration. For immediate removal, we'd need
	// a secondary index. For now, log and rely on eventual consistency.
	f.log.Info("Post deleted from global index - feed caches will expire",
		logger.Int64("post_id", postID))
	return nil
}

// GetFeed retrieves a user's feed with ranking and pagination.
func (f *FeedGenerator) GetFeed(ctx context.Context, userID int64, feedType string, cursor string, limit int) ([]FeedItem, string, bool, error) {
	ttl, ok := f.feedTTLs[feedType]
	if !ok {
		ttl = 5 * time.Minute
	}

	switch feedType {
	case "following":
		return f.getFollowingFeed(ctx, userID, cursor, limit, ttl)
	case "global":
		return f.getGlobalFeed(ctx, userID, cursor, limit, ttl)
	case "trending":
		return f.getTrendingFeed(ctx, userID, cursor, limit, ttl)
	default:
		return f.getFollowingFeed(ctx, userID, cursor, limit, ttl)
	}
}

// ---- Push Mode (Fan-out) ----

func (f *FeedGenerator) pushMode(ctx context.Context, authorID int64, postID int64, followerCount int) error {
	f.log.Info("Using push mode for post",
		logger.Int64("post_id", postID),
		logger.Int64("author_id", authorID),
		logger.Int("followers", followerCount))

	// Get all followers
	followerIDs, err := f.getFollowerIDs(ctx, authorID)
	if err != nil {
		return fmt.Errorf("get followers for post %d: %w", postID, err)
	}

	if len(followerIDs) == 0 {
		f.log.Info("No followers to push feed to",
			logger.Int64("post_id", postID))
		return nil
	}

	// Calculate base score for the post (initial post has no engagement yet)
	baseScore := f.initialScore(time.Now().UTC())

	// Push to each follower's following feed
	successCount := 0
	for _, followerID := range followerIDs {
		if err := f.redis.AddToFeed(ctx, followerID, "following", postID, baseScore); err != nil {
			f.log.Error("Failed to push post to follower feed",
				logger.Int64("follower_id", followerID),
				logger.Int64("post_id", postID),
				logger.Error(err))
			continue
		}
		// Set TTL on the feed
		f.redis.Rdb().Expire(ctx, redispkg.FeedCacheKey(followerID, "following"), f.feedTTLs["following"])
		successCount++
	}

	f.log.Info("Push mode complete",
		logger.Int64("post_id", postID),
		logger.Int("total_followers", len(followerIDs)),
		logger.Int("successful_pushes", successCount))

	return nil
}

// ---- Pull Mode Feed Retrieval ----

func (f *FeedGenerator) getFollowingFeed(ctx context.Context, userID int64, cursor string, limit int, ttl time.Duration) ([]FeedItem, string, bool, error) {
	// Try Redis cache first
	items, nextCursor, hasMore, err := f.getFeedFromRedis(ctx, userID, "following", cursor, limit)
	if err == nil && len(items) > 0 {
		return items, nextCursor, hasMore, nil
	}

	// Fallback: generate from Regional DB + Global Index
	f.log.Info("Feed cache miss, generating from source",
		logger.Int64("user_id", userID),
		logger.String("feed_type", "following"))

	// Get IDs of users this user follows
	followingIDs, err := f.getFollowingIDs(ctx, userID)
	if err != nil {
		return nil, "", false, fmt.Errorf("get following IDs for user %d: %w", userID, err)
	}

	if len(followingIDs) == 0 {
		return []FeedItem{}, "", false, nil
	}

	// Query global index for posts from followed users
	posts, err := f.indexSvc.GetPostsFromAuthors(ctx, followingIDs, limit)
	if err != nil {
		return nil, "", false, fmt.Errorf("query global index: %w", err)
	}

	items = f.toFeedItems(ctx, posts, userID)
	if err := f.cacheFeedItems(ctx, userID, "following", items, ttl); err != nil {
		f.log.Warn("Failed to cache following feed", logger.Error(err))
	}

	hasMore = len(posts) >= limit
	nextCursor = ""
	if hasMore && len(items) > 0 {
		nextCursor = fmt.Sprintf("%d", items[len(items)-1].PostID)
	}

	return items, nextCursor, hasMore, nil
}

func (f *FeedGenerator) getGlobalFeed(ctx context.Context, userID int64, cursor string, limit int, ttl time.Duration) ([]FeedItem, string, bool, error) {
	// Try Redis cache first
	items, nextCursor, hasMore, err := f.getFeedFromRedis(ctx, userID, "global", cursor, limit)
	if err == nil && len(items) > 0 {
		return items, nextCursor, hasMore, nil
	}

	f.log.Info("Global feed cache miss, generating from source",
		logger.Int64("user_id", userID))

	// Query global index for recent public posts
	posts, err := f.indexSvc.GetGlobalPosts(ctx, limit)
	if err != nil {
		return nil, "", false, fmt.Errorf("query global index: %w", err)
	}

	items = f.toFeedItems(ctx, posts, userID)
	if err := f.cacheFeedItems(ctx, userID, "global", items, ttl); err != nil {
		f.log.Warn("Failed to cache global feed", logger.Error(err))
	}

	hasMore = len(posts) >= limit
	nextCursor = ""
	if hasMore && len(items) > 0 {
		nextCursor = fmt.Sprintf("%d", items[len(items)-1].PostID)
	}

	return items, nextCursor, hasMore, nil
}

func (f *FeedGenerator) getTrendingFeed(ctx context.Context, userID int64, cursor string, limit int, ttl time.Duration) ([]FeedItem, string, bool, error) {
	// Try Redis cache first
	items, nextCursor, hasMore, err := f.getFeedFromRedis(ctx, userID, "trending", cursor, limit)
	if err == nil && len(items) > 0 {
		return items, nextCursor, hasMore, nil
	}

	f.log.Info("Trending feed cache miss, generating from source",
		logger.Int64("user_id", userID))

	// Query global index for posts with highest engagement
	posts, err := f.indexSvc.GetTrendingPosts(ctx, limit)
	if err != nil {
		return nil, "", false, fmt.Errorf("query global index: %w", err)
	}

	items = f.toFeedItems(ctx, posts, userID)
	if err := f.cacheFeedItems(ctx, userID, "trending", items, ttl); err != nil {
		f.log.Warn("Failed to cache trending feed", logger.Error(err))
	}

	hasMore = len(posts) >= limit
	nextCursor = ""
	if hasMore && len(items) > 0 {
		nextCursor = fmt.Sprintf("%d", items[len(items)-1].PostID)
	}

	return items, nextCursor, hasMore, nil
}

// ---- Redis Helpers ----

func (f *FeedGenerator) getFeedFromRedis(ctx context.Context, userID int64, feedType string, cursor string, limit int) ([]FeedItem, string, bool, error) {
	var offset int64 = 0
	if cursor != "" {
		// Parse cursor as offset
		// In production, cursor would encode offset
		_ = cursor // simplified for now
	}

	members, err := f.redis.GetFeed(ctx, userID, feedType, offset, int64(limit))
	if err != nil {
		return nil, "", false, err
	}

	items := make([]FeedItem, 0, len(members))
	for _, m := range members {
		var postID int64
		switch v := m.Member.(type) {
		case int64:
			postID = v
		case string:
			// Redis ZRANGE returns members as strings, not int64
			if id, err := strconv.ParseInt(v, 10, 64); err == nil {
				postID = id
			}
		default:
			f.log.Warn("Unknown member type in feed ZSET",
				logger.String("feed_type", feedType),
				logger.Any("member_type", fmt.Sprintf("%T", m.Member)))
			continue
		}
		items = append(items, FeedItem{
			PostID: postID,
			Score:  m.Score,
		})
	}

	hasMore := len(members) >= limit
	nextCursor := ""
	if hasMore && len(items) > 0 {
		nextCursor = fmt.Sprintf("%d", offset+int64(len(members)))
	}

	return items, nextCursor, hasMore, nil
}

func (f *FeedGenerator) cacheFeedItems(ctx context.Context, userID int64, feedType string, items []FeedItem, ttl time.Duration) error {
	key := redispkg.FeedCacheKey(userID, feedType)
	f.redis.Rdb().Del(ctx, key)

	members := make([]redis.Z, len(items))
	for i, item := range items {
		members[i] = redis.Z{
			Score:  item.Score,
			Member: item.PostID,
		}
	}
	f.redis.Rdb().ZAdd(ctx, key, members...)
	f.redis.Rdb().Expire(ctx, key, ttl)
	return nil
}

// ---- Ranking Algorithm ----

// RankingScore represents the components of a post's relevance score.
type RankingScore struct {
	Total      float64
	TimeDecay  float64
	Engagement float64
	Affinity   float64
	Preference float64
}

// CalculateScore computes the relevance score for a post for a given user.
//
// Formula: total = timeDecay*0.30 + engagementRate*0.30 + affinity*0.30 + preference*0.10
//
// Time decay: exponential with 6-hour half-life
// Engagement rate: (likes*1 + comments*2 + shares*3) / views^0.5
// Affinity: based on follow relationship (simplified)
// Preference: based on hashtag overlap (simplified)
func (f *FeedGenerator) CalculateScore(createdAt time.Time, likes, comments, shares, views int, isFollowing bool, tagOverlap float64) RankingScore {
	now := time.Now().UTC()
	hoursSincePost := now.Sub(createdAt).Hours()
	if hoursSincePost < 0 {
		hoursSincePost = 0
	}

	// Time decay: exponential decay with 6-hour half-life
	timeDecay := math.Exp(-0.693 * hoursSincePost / 6.0)

	// Engagement rate: normalized to 0-1
	var engagement float64
	if views > 0 {
		engagement = float64(likes*1+comments*2+shares*3) / math.Sqrt(float64(views))
	}
	// Clamp to 0-1
	if engagement > 1 {
		engagement = 1
	}

	// Affinity: 1.0 if following, 0.5 otherwise
	affinity := 0.5
	if isFollowing {
		affinity = 1.0
	}

	// Preference: hashtag overlap score (0-1)
	preference := tagOverlap
	if preference > 1 {
		preference = 1
	}

	// Weighted total
	total := timeDecay*0.30 + engagement*0.30 + affinity*0.30 + preference*0.10

	return RankingScore{
		Total:      total,
		TimeDecay:  timeDecay,
		Engagement: engagement,
		Affinity:   affinity,
		Preference: preference,
	}
}

func (f *FeedGenerator) initialScore(createdAt time.Time) float64 {
	score := f.CalculateScore(createdAt, 0, 0, 0, 0, false, 0)
	return score.Total
}

// ---- Data Helpers ----

func (f *FeedGenerator) getFollowerCount(ctx context.Context, userID int64) (int, error) {
	var count int
	query := `SELECT followers_count FROM users WHERE user_id = $1`
	err := f.regionalDB.RegionalDB().QueryRow(ctx, query, userID).Scan(&count)
	if err == pgx.ErrNoRows {
		// User not yet in Regional DB (managed by wigowago-api migrations).
		// Safe fallback to pull mode — not an error.
		return 0, nil
	}
	return count, err
}

func (f *FeedGenerator) getFollowerIDs(ctx context.Context, userID int64) ([]int64, error) {
	query := `SELECT follower_id FROM user_follows WHERE following_id = $1`
	rows, err := f.regionalDB.RegionalDB().Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (f *FeedGenerator) getFollowingIDs(ctx context.Context, userID int64) ([]int64, error) {
	query := `SELECT following_id FROM user_follows WHERE follower_id = $1`
	rows, err := f.regionalDB.RegionalDB().Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// FeedItem represents an item in a user's feed response.
type FeedItem struct {
	PostID         int64   `json:"postId"`
	AuthorID       int64   `json:"authorId"`
	AuthorName     string  `json:"authorName,omitempty"`
	ContentPreview string  `json:"contentPreview,omitempty"`
	Score          float64 `json:"score"`
	Engagement     struct {
		Likes    int `json:"likes"`
		Comments int `json:"comments"`
		Shares   int `json:"shares"`
	} `json:"engagement"`
}

func (f *FeedGenerator) toFeedItems(ctx context.Context, posts []GlobalIndexPost, _ int64) []FeedItem {
	items := make([]FeedItem, 0, len(posts))
	for _, p := range posts {
		item := FeedItem{
			PostID:         p.PostID,
			AuthorID:       p.AuthorID,
			ContentPreview: p.ContentPreview,
		}
		item.Engagement.Likes = p.LikesCount
		item.Engagement.Comments = p.CommentsCount
		item.Engagement.Shares = p.SharesCount
		score := f.CalculateScore(p.CreatedAt, p.LikesCount, p.CommentsCount, p.SharesCount, p.ViewsCount, false, 0)
		item.Score = score.Total
		items = append(items, item)
	}
	return items
}
