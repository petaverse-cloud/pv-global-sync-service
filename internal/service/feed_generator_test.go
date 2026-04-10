package service

import (
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
