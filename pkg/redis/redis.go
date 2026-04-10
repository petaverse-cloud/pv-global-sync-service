// Package redis provides a Redis client wrapper for the Global Sync Service.
// Uses Redis Stack capabilities (JSON, Search, TimeSeries).
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps the go-redis client with service-specific methods
type Client struct {
	rdb *redis.Client
}

// Config holds Redis connection configuration
type Config struct {
	Host     string
	Port     int
	Password string
	DB       int
}

// Addr returns the Redis server address
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// New creates a new Redis client and verifies connectivity
func New(ctx context.Context, cfg Config) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr(),
		Password:     cfg.Password,
		DB:           cfg.DB,
		MinIdleConns: 5,
		PoolSize:     20,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return &Client{rdb: rdb}, nil
}

// Rdb returns the underlying redis.Client for advanced operations
func (c *Client) Rdb() *redis.Client {
	return c.rdb
}

// Ping checks Redis connectivity
func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// Close closes the Redis connection
func (c *Client) Close() error {
	return c.rdb.Close()
}

// ---- Feed Cache Operations ----

// FeedCacheKey returns the Redis key for a user's feed
func FeedCacheKey(userID int64, feedType string) string {
	return fmt.Sprintf("user:feed:%d:%s", userID, feedType)
}

// AddToFeed adds a post to a user's feed ZSET with score
func (c *Client) AddToFeed(ctx context.Context, userID int64, feedType string, postID int64, score float64) error {
	return c.rdb.ZAdd(ctx, FeedCacheKey(userID, feedType), redis.Z{
		Score:  score,
		Member: postID,
	}).Err()
}

// GetFeed retrieves feed items with pagination (by score range)
func (c *Client) GetFeed(ctx context.Context, userID int64, feedType string, offset, limit int64) ([]redis.Z, error) {
	// Get by rank (offset-based pagination on sorted set)
	return c.rdb.ZRevRangeWithScores(ctx, FeedCacheKey(userID, feedType), offset, offset+limit-1).Result()
}

// SetFeedTTL sets expiration on a feed cache key
func (c *Client) SetFeedTTL(ctx context.Context, userID int64, feedType string, ttl time.Duration) error {
	return c.rdb.Expire(ctx, FeedCacheKey(userID, feedType), ttl).Err()
}

// DeleteFeed removes a user's feed cache
func (c *Client) DeleteFeed(ctx context.Context, userID int64, feedType string) error {
	return c.rdb.Del(ctx, FeedCacheKey(userID, feedType)).Err()
}

// ---- Post Cache Operations ----

// PostCacheKey returns the Redis key for post detail cache
func PostCacheKey(postID int64) string {
	return fmt.Sprintf("post:%d", postID)
}

// SetPost caches post detail as JSON
func (c *Client) SetPost(ctx context.Context, postID int64, data string, ttl time.Duration) error {
	return c.rdb.Set(ctx, PostCacheKey(postID), data, ttl).Err()
}

// GetPost retrieves cached post detail
func (c *Client) GetPost(ctx context.Context, postID int64) (string, error) {
	return c.rdb.Get(ctx, PostCacheKey(postID)).Result()
}

// ---- Event Deduplication ----

// IsEventProcessed checks if an event has already been processed
func (c *Client) IsEventProcessed(ctx context.Context, eventID string) (bool, error) {
	exists, err := c.rdb.SIsMember(ctx, "sync:event:processed", eventID).Result()
	return exists, err
}

// MarkEventProcessed marks an event as processed with TTL
func (c *Client) MarkEventProcessed(ctx context.Context, eventID string) error {
	pipe := c.rdb.TxPipeline()
	pipe.SAdd(ctx, "sync:event:processed", eventID)
	pipe.Expire(ctx, "sync:event:processed", 24*time.Hour)
	_, err := pipe.Exec(ctx)
	return err
}
