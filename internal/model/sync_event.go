package model

import "time"

// SyncEventType represents the type of cross-region sync event
type SyncEventType string

const (
	EventTypePostCreated      SyncEventType = "POST_CREATED"
	EventTypePostUpdated      SyncEventType = "POST_UPDATED"
	EventTypePostDeleted      SyncEventType = "POST_DELETED"
	EventTypePostStatsUpdated SyncEventType = "POST_STATS_UPDATED"
)

// Visibility represents post visibility level
type Visibility string

const (
	VisibilityGlobal    Visibility = "GLOBAL"
	VisibilityRegional  Visibility = "REGIONAL"
	VisibilityFollowers Visibility = "FOLLOWERS"
	VisibilityPrivate   Visibility = "PRIVATE"
)

// DataCategory represents GDPR data tier
type DataCategory string

const (
	DataCategoryPII    DataCategory = "TIER_1" // Personal Identifiable Information
	DataCategoryUGC    DataCategory = "TIER_2" // User Generated Content
	DataCategorySystem DataCategory = "TIER_3" // System data
	DataCategoryMedia  DataCategory = "TIER_4" // Media files
)

// Region represents deployment region
type Region string

const (
	RegionEU  Region = "EU"
	RegionNA  Region = "NA"
	RegionSEA Region = "SEA" // Southeast Asia
)

// CrossRegionSyncEvent represents a cross-region synchronization event
// Aligned with wigowago-v2-distributed-architecture.md event definition
type CrossRegionSyncEvent struct {
	EventID      string        `json:"eventId"`
	EventType    SyncEventType `json:"eventType"`
	SourceRegion Region        `json:"sourceRegion"`
	TargetRegion Region        `json:"targetRegion"`
	Timestamp    int64         `json:"timestamp"`
	Payload      EventPayload  `json:"payload"`
	Metadata     EventMetadata `json:"metadata"`
}

// EventPayload contains the core data of the sync event
type EventPayload struct {
	PostID       int64      `json:"postId"`
	AuthorID     int64      `json:"authorId"`
	AuthorRegion Region     `json:"authorRegion"`
	Visibility   Visibility `json:"visibility"`
	Content      string     `json:"content,omitempty"`
	MediaURLs    []string   `json:"mediaUrls,omitempty"`
	// Author Metadata (Layer 1: Public Info)
	AuthorProfile *AuthorProfile `json:"authorProfile,omitempty"`
}

// AuthorProfile contains public author info for feed display.
type AuthorProfile struct {
	Slug      int64  `json:"slug"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatarUrl"`
}

// EventMetadata contains compliance and audit information
type EventMetadata struct {
	GDPRCompliant bool         `json:"gdprCompliant"`
	UserConsent   bool         `json:"userConsent"`
	DataCategory  DataCategory `json:"dataCategory"`
	CrossBorderOK bool         `json:"crossBorderOk"`
}

// GlobalPostIndex represents a post entry in the global index
type GlobalPostIndex struct {
	PostID         int64     `json:"postId"`
	AuthorID       int64     `json:"authorId"`
	AuthorRegion   Region    `json:"authorRegion"`
	ContentPreview string    `json:"contentPreview"`
	Visibility     string    `json:"visibility"`
	Hashtags       []string  `json:"hashtags,omitempty"`
	Mentions       []int64   `json:"mentions,omitempty"`
	MediaURLs      []string  `json:"mediaUrls,omitempty"`
	LikesCount     int       `json:"likesCount"`
	CommentsCount  int       `json:"commentsCount"`
	SharesCount    int       `json:"sharesCount"`
	ViewsCount     int       `json:"viewsCount"`
	GDPRCompliant  bool      `json:"gdprCompliant"`
	UserConsent    bool      `json:"userConsent"`
	DataCategory   string    `json:"dataCategory"`
	CreatedAt      time.Time `json:"createdAt"`
	SyncedAt       time.Time `json:"syncedAt"`
	// Author Metadata (Layer 1: Public Info)
	AuthorSlug      *int64 `json:"authorSlug,omitempty"`
	AuthorNickname  string `json:"authorNickname"`
	AuthorAvatarURL string `json:"authorAvatarUrl"`
}

// FeedItem represents an item in a user's feed
type FeedItem struct {
	UserID    int64     `json:"userId"`
	PostID    int64     `json:"postId"`
	FeedType  string    `json:"feedType"` // "following" | "global" | "trending"
	Score     float64   `json:"score"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}
