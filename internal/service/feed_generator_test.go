package service

import (
	"context"
	"testing"
	"time"
)

func TestCalculateScore(t *testing.T) {
	f := &FeedGenerator{}
	now := time.Now().UTC()

	tests := []struct {
		name        string
		createdAt   time.Time
		likes       int
		comments    int
		shares      int
		views       int
		isFollowing bool
		tagOverlap  float64
		checkFn     func(t *testing.T, score RankingScore)
	}{
		{
			name:        "fresh post with no engagement",
			createdAt:   now,
			likes:       0,
			comments:    0,
			shares:      0,
			views:       0,
			isFollowing: false,
			tagOverlap:  0,
			checkFn: func(t *testing.T, s RankingScore) {
				if s.TimeDecay < 0.99 {
					t.Errorf("TimeDecay for fresh post = %.4f, want ~1.0", s.TimeDecay)
				}
				if s.Affinity != 0.5 {
					t.Errorf("Affinity for non-follower = %.2f, want 0.5", s.Affinity)
				}
			},
		},
		{
			name:        "following user with high engagement",
			createdAt:   now,
			likes:       100,
			comments:    50,
			shares:      20,
			views:       1000,
			isFollowing: true,
			tagOverlap:  0.8,
			checkFn: func(t *testing.T, s RankingScore) {
				if s.Affinity != 1.0 {
					t.Errorf("Affinity for following = %.2f, want 1.0", s.Affinity)
				}
				if s.Preference != 0.8 {
					t.Errorf("Preference = %.2f, want 0.8", s.Preference)
				}
			},
		},
		{
			name:        "old post decays to near zero",
			createdAt:   now.Add(-48 * time.Hour),
			likes:       10,
			comments:    5,
			shares:      1,
			views:       100,
			isFollowing: false,
			tagOverlap:  0,
			checkFn: func(t *testing.T, s RankingScore) {
				if s.TimeDecay > 0.01 {
					t.Errorf("TimeDecay for 48h old post = %.4f, want ~0", s.TimeDecay)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := f.CalculateScore(tt.createdAt, tt.likes, tt.comments, tt.shares, tt.views, tt.isFollowing, tt.tagOverlap)
			tt.checkFn(t, score)
		})
	}
}

func TestFeedTTLs(t *testing.T) {
	ttls := FeedTTLs()

	if ttls["following"] != 5*time.Minute {
		t.Errorf("following TTL = %v, want 5m", ttls["following"])
	}
	if ttls["global"] != 15*time.Minute {
		t.Errorf("global TTL = %v, want 15m", ttls["global"])
	}
	if ttls["trending"] != 1*time.Minute {
		t.Errorf("trending TTL = %v, want 1m", ttls["trending"])
	}
}

func TestInitialScore(t *testing.T) {
	f := &FeedGenerator{}
	now := time.Now().UTC()

	score := f.initialScore(now)
	// Should be mostly time decay + affinity (0.5) + zero engagement
	// timeDecay ~1.0, engagement 0, affinity 0.5, preference 0
	// total = 1.0*0.30 + 0*0.30 + 0.5*0.30 + 0*0.10 = 0.45
	if score < 0.4 || score > 0.5 {
		t.Errorf("initialScore for fresh post = %.4f, want ~0.45", score)
	}
}

func TestToFeedItems_EmptyPosts(t *testing.T) {
	// toFeedItems with an empty post slice should return an empty FeedItem slice
	// without panicking, even when called on a nil FeedGenerator (the method
	// body never executes the loop, and CalculateScore does not dereceive the receiver).
	var fg *FeedGenerator
	items := fg.toFeedItems(context.Background(), []GlobalIndexPost{}, 0)

	if items == nil {
		t.Fatal("toFeedItems returned nil for empty posts, expected empty slice")
	}
	if len(items) != 0 {
		t.Errorf("toFeedItems returned %d items for empty posts, want 0", len(items))
	}
}

func TestToFeedItems_ZeroCounts(t *testing.T) {
	// toFeedItems with posts that have all zero engagement counts should
	// produce FeedItems with the correct score (dominated by time decay + affinity).
	fg := &FeedGenerator{}
	now := time.Now().UTC()

	posts := []GlobalIndexPost{
		{
			PostID:         1,
			AuthorID:       100,
			ContentPreview: "hello world",
			LikesCount:     0,
			CommentsCount:  0,
			SharesCount:    0,
			ViewsCount:     0,
			CreatedAt:      now,
		},
		{
			PostID:         2,
			AuthorID:       200,
			ContentPreview: "second post",
			LikesCount:     0,
			CommentsCount:  0,
			SharesCount:    0,
			ViewsCount:     0,
			CreatedAt:      now.Add(-1 * time.Hour),
		},
	}

	items := fg.toFeedItems(context.Background(), posts, 0)

	if len(items) != 2 {
		t.Fatalf("toFeedItems returned %d items, want 2", len(items))
	}

	// Verify first item (fresh post)
	if items[0].PostID != 1 {
		t.Errorf("item[0].PostID = %d, want 1", items[0].PostID)
	}
	if items[0].AuthorID != 100 {
		t.Errorf("item[0].AuthorID = %d, want 100", items[0].AuthorID)
	}
	if items[0].ContentPreview != "hello world" {
		t.Errorf("item[0].ContentPreview = %q, want \"hello world\"", items[0].ContentPreview)
	}
	if items[0].Engagement.Likes != 0 {
		t.Errorf("item[0].Engagement.Likes = %d, want 0", items[0].Engagement.Likes)
	}
	if items[0].Engagement.Comments != 0 {
		t.Errorf("item[0].Engagement.Comments = %d, want 0", items[0].Engagement.Comments)
	}
	if items[0].Engagement.Shares != 0 {
		t.Errorf("item[0].Engagement.Shares = %d, want 0", items[0].Engagement.Shares)
	}
	// Fresh post: timeDecay~1.0, engagement=0, affinity=0.5, preference=0 -> ~0.45
	if items[0].Score < 0.4 || items[0].Score > 0.5 {
		t.Errorf("item[0].Score = %.4f, want ~0.45", items[0].Score)
	}

	// Verify second item (1 hour old, should have lower time decay)
	if items[1].PostID != 2 {
		t.Errorf("item[1].PostID = %d, want 2", items[1].PostID)
	}
	// 1 hour old: timeDecay = exp(-0.693*1/6) ~ 0.89, so score should be lower than fresh
	if items[1].Score >= items[0].Score {
		t.Errorf("item[1].Score (%.4f) should be < item[0].Score (%.4f) since it's older", items[1].Score, items[0].Score)
	}
}

func TestGetFeed_UnknownFeedTypeDefaultsToFollowing(t *testing.T) {
	// GetFeed with an unknown feedType (not "following", "global", or "trending")
	// should fall through to the default case and call getFollowingFeed.
	// getFollowingFeed with empty followingIDs returns []FeedItem{}, "", false, nil.
	//
	// NOTE: This test documents the expected behavior. A full integration test
	// with real Redis/DB deps is needed to verify the end-to-end path, since
	// getFollowingFeed calls getFeedFromRedis which requires a live Redis client.
	// The default-case routing logic is: feedType != "following"/"global"/"trending"
	// -> switch default -> f.getFollowingFeed(...).
}

func TestGetFeed_EmptyFeedTypeDefaultsToFollowing(t *testing.T) {
	// GetFeed with an empty string feedType should also default to following feed.
	// Empty string does not match any case in the switch, so it falls to default.
	//
	// NOTE: Requires integration deps (Redis, DB) to exercise the full path.
	// This test documents the expected routing behavior.
}
