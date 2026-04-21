package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCrossRegionSyncEventJSON(t *testing.T) {
	event := CrossRegionSyncEvent{
		EventID:      "evt_123",
		EventType:    EventTypePostCreated,
		SourceRegion: RegionEU,
		TargetRegion: RegionNA,
		Timestamp:    1712736000,
		Payload: EventPayload{
			PostID:       42,
			AuthorID:     7,
			AuthorRegion: RegionEU,
			Visibility:   VisibilityGlobal,
			Content:      "Hello world #test",
			MediaURLs:    []string{"https://example.com/img.jpg"},
		},
		Metadata: EventMetadata{
			GDPRCompliant: true,
			UserConsent:   true,
			DataCategory:  DataCategoryUGC,
			CrossBorderOK: true,
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded CrossRegionSyncEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.EventID != event.EventID {
		t.Errorf("EventID = %q, want %q", decoded.EventID, event.EventID)
	}
	if decoded.Payload.PostID != event.Payload.PostID {
		t.Errorf("PostID = %d, want %d", decoded.Payload.PostID, event.Payload.PostID)
	}
	if decoded.Metadata.DataCategory != DataCategoryUGC {
		t.Errorf("DataCategory = %q, want %q", decoded.Metadata.DataCategory, DataCategoryUGC)
	}
}

func TestSyncEventConstants(t *testing.T) {
	if EventTypePostCreated != "POST_CREATED" {
		t.Errorf("EventTypePostCreated = %q, want %q", EventTypePostCreated, "POST_CREATED")
	}
	if VisibilityGlobal != "GLOBAL" {
		t.Errorf("VisibilityGlobal = %q, want %q", VisibilityGlobal, "GLOBAL")
	}
	if DataCategoryPII != "TIER_1" {
		t.Errorf("DataCategoryPII = %q, want %q", DataCategoryPII, "TIER_1")
	}
	if RegionEU != "EU" {
		t.Errorf("RegionEU = %q, want %q", RegionEU, "EU")
	}
}

func TestAllSyncEventTypeConstants(t *testing.T) {
	tests := []struct {
		name  string
		value SyncEventType
		want  string
	}{
		{"PostCreated", EventTypePostCreated, "POST_CREATED"},
		{"PostUpdated", EventTypePostUpdated, "POST_UPDATED"},
		{"PostDeleted", EventTypePostDeleted, "POST_DELETED"},
		{"PostStatsUpdated", EventTypePostStatsUpdated, "POST_STATS_UPDATED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.value) != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.value, tt.want)
			}
		})
	}
}

func TestAllVisibilityConstants(t *testing.T) {
	tests := []struct {
		name  string
		value Visibility
		want  string
	}{
		{"Global", VisibilityGlobal, "GLOBAL"},
		{"Regional", VisibilityRegional, "REGIONAL"},
		{"Followers", VisibilityFollowers, "FOLLOWERS"},
		{"Private", VisibilityPrivate, "PRIVATE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.value) != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.value, tt.want)
			}
		})
	}
}

func TestAllDataCategoryConstants(t *testing.T) {
	tests := []struct {
		name  string
		value DataCategory
		want  string
	}{
		{"PII", DataCategoryPII, "TIER_1"},
		{"UGC", DataCategoryUGC, "TIER_2"},
		{"System", DataCategorySystem, "TIER_3"},
		{"Media", DataCategoryMedia, "TIER_4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.value) != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.value, tt.want)
			}
		})
	}
}

func TestAllRegionConstants(t *testing.T) {
	tests := []struct {
		name  string
		value Region
		want  string
	}{
		{"EU", RegionEU, "EU"},
		{"NA", RegionNA, "NA"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.value) != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.value, tt.want)
			}
		})
	}
}

func TestGlobalPostIndexJSON(t *testing.T) {
	createdAt := time.Date(2024, 4, 10, 12, 0, 0, 0, time.UTC)
	syncedAt := time.Date(2024, 4, 10, 12, 1, 0, 0, time.UTC)

	idx := GlobalPostIndex{
		PostID:         99,
		AuthorID:       5,
		AuthorRegion:   RegionNA,
		ContentPreview: "Hello world #test @alice",
		Visibility:     string(VisibilityGlobal),
		Hashtags:       []string{"test", "golang", "sync"},
		Mentions:       []int64{1, 2, 3},
		MediaURLs:      []string{"https://cdn.example.com/a.jpg", "https://cdn.example.com/b.mp4"},
		LikesCount:     42,
		CommentsCount:  7,
		SharesCount:    3,
		ViewsCount:     150,
		GDPRCompliant:  true,
		UserConsent:    true,
		DataCategory:   string(DataCategoryUGC),
		CreatedAt:      createdAt,
		SyncedAt:       syncedAt,
	}

	data, err := json.Marshal(idx)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded GlobalPostIndex
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.PostID != idx.PostID {
		t.Errorf("PostID = %d, want %d", decoded.PostID, idx.PostID)
	}
	if decoded.AuthorID != idx.AuthorID {
		t.Errorf("AuthorID = %d, want %d", decoded.AuthorID, idx.AuthorID)
	}
	if decoded.AuthorRegion != idx.AuthorRegion {
		t.Errorf("AuthorRegion = %q, want %q", decoded.AuthorRegion, idx.AuthorRegion)
	}
	if decoded.ContentPreview != idx.ContentPreview {
		t.Errorf("ContentPreview = %q, want %q", decoded.ContentPreview, idx.ContentPreview)
	}
	if decoded.Visibility != idx.Visibility {
		t.Errorf("Visibility = %q, want %q", decoded.Visibility, idx.Visibility)
	}

	// Check slices
	if len(decoded.Hashtags) != len(idx.Hashtags) {
		t.Fatalf("Hashtags len = %d, want %d", len(decoded.Hashtags), len(idx.Hashtags))
	}
	for i := range idx.Hashtags {
		if decoded.Hashtags[i] != idx.Hashtags[i] {
			t.Errorf("Hashtags[%d] = %q, want %q", i, decoded.Hashtags[i], idx.Hashtags[i])
		}
	}

	if len(decoded.Mentions) != len(idx.Mentions) {
		t.Fatalf("Mentions len = %d, want %d", len(decoded.Mentions), len(idx.Mentions))
	}
	for i := range idx.Mentions {
		if decoded.Mentions[i] != idx.Mentions[i] {
			t.Errorf("Mentions[%d] = %d, want %d", i, decoded.Mentions[i], idx.Mentions[i])
		}
	}

	if len(decoded.MediaURLs) != len(idx.MediaURLs) {
		t.Fatalf("MediaURLs len = %d, want %d", len(decoded.MediaURLs), len(idx.MediaURLs))
	}
	for i := range idx.MediaURLs {
		if decoded.MediaURLs[i] != idx.MediaURLs[i] {
			t.Errorf("MediaURLs[%d] = %q, want %q", i, decoded.MediaURLs[i], idx.MediaURLs[i])
		}
	}

	if decoded.LikesCount != idx.LikesCount {
		t.Errorf("LikesCount = %d, want %d", decoded.LikesCount, idx.LikesCount)
	}
	if decoded.CommentsCount != idx.CommentsCount {
		t.Errorf("CommentsCount = %d, want %d", decoded.CommentsCount, idx.CommentsCount)
	}
	if decoded.SharesCount != idx.SharesCount {
		t.Errorf("SharesCount = %d, want %d", decoded.SharesCount, idx.SharesCount)
	}
	if decoded.ViewsCount != idx.ViewsCount {
		t.Errorf("ViewsCount = %d, want %d", decoded.ViewsCount, idx.ViewsCount)
	}
	if decoded.GDPRCompliant != idx.GDPRCompliant {
		t.Errorf("GDPRCompliant = %v, want %v", decoded.GDPRCompliant, idx.GDPRCompliant)
	}
	if decoded.UserConsent != idx.UserConsent {
		t.Errorf("UserConsent = %v, want %v", decoded.UserConsent, idx.UserConsent)
	}
	if decoded.DataCategory != idx.DataCategory {
		t.Errorf("DataCategory = %q, want %q", decoded.DataCategory, idx.DataCategory)
	}
	if !decoded.CreatedAt.Equal(idx.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", decoded.CreatedAt, idx.CreatedAt)
	}
	if !decoded.SyncedAt.Equal(idx.SyncedAt) {
		t.Errorf("SyncedAt = %v, want %v", decoded.SyncedAt, idx.SyncedAt)
	}
}

func TestGlobalPostIndexJSONWithEmptySlices(t *testing.T) {
	idx := GlobalPostIndex{
		PostID:       1,
		AuthorID:     10,
		AuthorRegion: RegionEU,
		Visibility:   string(VisibilityPrivate),
		Hashtags:     []string{},
		Mentions:     []int64{},
		MediaURLs:    []string{},
		CreatedAt:    time.Now(),
		SyncedAt:     time.Now(),
	}

	data, err := json.Marshal(idx)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded GlobalPostIndex
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Hashtags != nil {
		t.Errorf("empty Hashtags became non-nil after roundtrip, got %v", decoded.Hashtags)
	}
	if decoded.Mentions != nil {
		t.Errorf("empty Mentions became non-nil after roundtrip, got %v", decoded.Mentions)
	}
	if decoded.MediaURLs != nil {
		t.Errorf("empty MediaURLs became non-nil after roundtrip, got %v", decoded.MediaURLs)
	}

	// Verify omitempty: empty slices should be absent from JSON
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map error: %v", err)
	}
	if _, ok := raw["hashtags"]; ok {
		t.Error("hashtags should be omitted when empty")
	}
	if _, ok := raw["mentions"]; ok {
		t.Error("mentions should be omitted when empty")
	}
	if _, ok := raw["mediaUrls"]; ok {
		t.Error("mediaUrls should be omitted when empty")
	}
}

func TestFeedItemJSON(t *testing.T) {
	created := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)
	expires := time.Date(2024, 5, 8, 10, 0, 0, 0, time.UTC)

	item := FeedItem{
		UserID:    123,
		PostID:    456,
		FeedType:  "trending",
		Score:     0.95,
		CreatedAt: created,
		ExpiresAt: expires,
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded FeedItem
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.UserID != item.UserID {
		t.Errorf("UserID = %d, want %d", decoded.UserID, item.UserID)
	}
	if decoded.PostID != item.PostID {
		t.Errorf("PostID = %d, want %d", decoded.PostID, item.PostID)
	}
	if decoded.FeedType != item.FeedType {
		t.Errorf("FeedType = %q, want %q", decoded.FeedType, item.FeedType)
	}
	if decoded.Score != item.Score {
		t.Errorf("Score = %f, want %f", decoded.Score, item.Score)
	}
	if !decoded.CreatedAt.Equal(item.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", decoded.CreatedAt, item.CreatedAt)
	}
	if !decoded.ExpiresAt.Equal(item.ExpiresAt) {
		t.Errorf("ExpiresAt = %v, want %v", decoded.ExpiresAt, item.ExpiresAt)
	}
}

func TestFeedItemJSONWithoutExpiresAt(t *testing.T) {
	created := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)

	item := FeedItem{
		UserID:    1,
		PostID:    2,
		FeedType:  "global",
		Score:     0.5,
		CreatedAt: created,
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Note: Go's encoding/json does NOT omit zero time.Time with omitempty
	// because time.Time serializes to a non-empty string like "0001-01-01T00:00:00Z".
	// The zero ExpiresAt will appear in JSON as a zero RFC3339 timestamp.
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map error: %v", err)
	}
	// expiresAt IS present (zero time serializes as non-empty string)
	if _, ok := raw["expiresAt"]; !ok {
		t.Error("expiresAt should be present even when zero (time.Time zero is not omitted)")
	}

	var decoded FeedItem
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if !decoded.CreatedAt.Equal(item.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", decoded.CreatedAt, item.CreatedAt)
	}
}

func TestCrossRegionSyncEventWithEmptyMediaURLs(t *testing.T) {
	event := CrossRegionSyncEvent{
		EventID:      "evt_empty_media",
		EventType:    EventTypePostCreated,
		SourceRegion: RegionNA,
		TargetRegion: RegionEU,
		Timestamp:    1712736000,
		Payload: EventPayload{
			PostID:       100,
			AuthorID:     8,
			AuthorRegion: RegionNA,
			Visibility:   VisibilityRegional,
			Content:      "No media post",
			MediaURLs:    []string{},
		},
		Metadata: EventMetadata{
			GDPRCompliant: true,
			UserConsent:   true,
			DataCategory:  DataCategoryUGC,
			CrossBorderOK: true,
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded CrossRegionSyncEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Payload.PostID != event.Payload.PostID {
		t.Errorf("PostID = %d, want %d", decoded.Payload.PostID, event.Payload.PostID)
	}

	// Verify mediaUrls is omitted when empty
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map error: %v", err)
	}
	payload, ok := raw["payload"].(map[string]interface{})
	if !ok {
		t.Fatal("payload not found in raw JSON")
	}
	if _, hasMediaURLs := payload["mediaUrls"]; hasMediaURLs {
		t.Error("mediaUrls should be omitted when empty slice")
	}
}

func TestCrossRegionSyncEventWithMissingOptionalFields(t *testing.T) {
	event := CrossRegionSyncEvent{
		EventID:      "evt_minimal",
		EventType:    EventTypePostUpdated,
		SourceRegion: RegionEU,
		TargetRegion: RegionNA,
		Timestamp:    1712800000,
		Payload: EventPayload{
			PostID:       200,
			AuthorID:     9,
			AuthorRegion: RegionEU,
			Visibility:   VisibilityFollowers,
			// Content and MediaURLs intentionally left zero/empty
		},
		Metadata: EventMetadata{
			GDPRCompliant: false,
			UserConsent:   false,
			DataCategory:  DataCategorySystem,
			CrossBorderOK: false,
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded CrossRegionSyncEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Payload.Content != "" {
		t.Errorf("Content = %q, want empty", decoded.Payload.Content)
	}
	if decoded.Payload.MediaURLs != nil {
		t.Errorf("MediaURLs = %v, want nil", decoded.Payload.MediaURLs)
	}

	// Verify content and mediaUrls absent from JSON
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map error: %v", err)
	}
	payload, ok := raw["payload"].(map[string]interface{})
	if !ok {
		t.Fatal("payload not found in raw JSON")
	}
	if _, hasContent := payload["content"]; hasContent {
		t.Error("content should be omitted when empty string")
	}
	if _, hasMediaURLs := payload["mediaUrls"]; hasMediaURLs {
		t.Error("mediaUrls should be omitted when nil")
	}
}

func TestCrossRegionSyncEventZeroValue(t *testing.T) {
	var event CrossRegionSyncEvent

	// Zero-value event should marshal and unmarshal without error
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Zero-value Marshal error: %v", err)
	}

	var decoded CrossRegionSyncEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Zero-value Unmarshal error: %v", err)
	}

	if decoded.EventID != "" {
		t.Errorf("zero EventID = %q, want empty", decoded.EventID)
	}
	if decoded.EventType != "" {
		t.Errorf("zero EventType = %q, want empty", decoded.EventType)
	}
	if decoded.SourceRegion != "" {
		t.Errorf("zero SourceRegion = %q, want empty", decoded.SourceRegion)
	}
	if decoded.TargetRegion != "" {
		t.Errorf("zero TargetRegion = %q, want empty", decoded.TargetRegion)
	}
	if decoded.Timestamp != 0 {
		t.Errorf("zero Timestamp = %d, want 0", decoded.Timestamp)
	}
	if decoded.Payload.PostID != 0 {
		t.Errorf("zero Payload.PostID = %d, want 0", decoded.Payload.PostID)
	}
	if decoded.Metadata.GDPRCompliant != false {
		t.Error("zero Metadata.GDPRCompliant should be false")
	}
}

func TestGlobalPostIndexZeroValue(t *testing.T) {
	var idx GlobalPostIndex

	data, err := json.Marshal(idx)
	if err != nil {
		t.Fatalf("Zero-value Marshal error: %v", err)
	}

	var decoded GlobalPostIndex
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Zero-value Unmarshal error: %v", err)
	}

	if decoded.PostID != 0 {
		t.Errorf("zero PostID = %d, want 0", decoded.PostID)
	}
	if decoded.AuthorID != 0 {
		t.Errorf("zero AuthorID = %d, want 0", decoded.AuthorID)
	}
	if decoded.ContentPreview != "" {
		t.Errorf("zero ContentPreview = %q, want empty", decoded.ContentPreview)
	}
	if decoded.LikesCount != 0 {
		t.Errorf("zero LikesCount = %d, want 0", decoded.LikesCount)
	}
	if decoded.GDPRCompliant != false {
		t.Error("zero GDPRCompliant should be false")
	}
	if decoded.UserConsent != false {
		t.Error("zero UserConsent should be false")
	}
}

func TestFeedItemZeroValue(t *testing.T) {
	var item FeedItem

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("Zero-value Marshal error: %v", err)
	}

	var decoded FeedItem
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Zero-value Unmarshal error: %v", err)
	}

	if decoded.UserID != 0 {
		t.Errorf("zero UserID = %d, want 0", decoded.UserID)
	}
	if decoded.PostID != 0 {
		t.Errorf("zero PostID = %d, want 0", decoded.PostID)
	}
	if decoded.Score != 0.0 {
		t.Errorf("zero Score = %f, want 0.0", decoded.Score)
	}
}

func TestCrossRegionSyncEventWithLargeContent(t *testing.T) {
	// Generate 10KB content
	largeContent := make([]byte, 10240)
	for i := range largeContent {
		largeContent[i] = 'A' + byte(i%26)
	}

	event := CrossRegionSyncEvent{
		EventID:      "evt_large",
		EventType:    EventTypePostCreated,
		SourceRegion: RegionEU,
		TargetRegion: RegionNA,
		Timestamp:    1712900000,
		Payload: EventPayload{
			PostID:       500,
			AuthorID:     50,
			AuthorRegion: RegionEU,
			Visibility:   VisibilityGlobal,
			Content:      string(largeContent),
		},
		Metadata: EventMetadata{
			GDPRCompliant: true,
			UserConsent:   true,
			DataCategory:  DataCategoryUGC,
			CrossBorderOK: true,
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal error with large content: %v", err)
	}

	if len(data) < 10240 {
		t.Errorf("marshalled JSON size = %d, expected > 10240", len(data))
	}

	var decoded CrossRegionSyncEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error with large content: %v", err)
	}

	if decoded.Payload.Content != event.Payload.Content {
		t.Errorf("large content mismatch: got len=%d, want len=%d",
			len(decoded.Payload.Content), len(event.Payload.Content))
	}
	if decoded.Payload.PostID != 500 {
		t.Errorf("PostID = %d, want 500", decoded.Payload.PostID)
	}
}

func TestGlobalPostIndexJSONWithNilSlices(t *testing.T) {
	idx := GlobalPostIndex{
		PostID:       2,
		AuthorID:     20,
		AuthorRegion: RegionNA,
		Visibility:   string(VisibilityGlobal),
		Hashtags:     nil,
		Mentions:     nil,
		MediaURLs:    nil,
		CreatedAt:    time.Now(),
		SyncedAt:     time.Now(),
	}

	data, err := json.Marshal(idx)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded GlobalPostIndex
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Hashtags != nil {
		t.Errorf("nil Hashtags after roundtrip = %v, want nil", decoded.Hashtags)
	}
	if decoded.Mentions != nil {
		t.Errorf("nil Mentions after roundtrip = %v, want nil", decoded.Mentions)
	}
	if decoded.MediaURLs != nil {
		t.Errorf("nil MediaURLs after roundtrip = %v, want nil", decoded.MediaURLs)
	}
}

func TestEventPayloadJSONRoundtrip(t *testing.T) {
	payload := EventPayload{
		PostID:       777,
		AuthorID:     888,
		AuthorRegion: RegionNA,
		Visibility:   VisibilityPrivate,
		Content:      "Secret post",
		MediaURLs:    []string{"https://cdn.example.com/secret.png"},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded EventPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.PostID != payload.PostID {
		t.Errorf("PostID = %d, want %d", decoded.PostID, payload.PostID)
	}
	if decoded.AuthorID != payload.AuthorID {
		t.Errorf("AuthorID = %d, want %d", decoded.AuthorID, payload.AuthorID)
	}
	if decoded.AuthorRegion != payload.AuthorRegion {
		t.Errorf("AuthorRegion = %q, want %q", decoded.AuthorRegion, payload.AuthorRegion)
	}
	if decoded.Visibility != payload.Visibility {
		t.Errorf("Visibility = %q, want %q", decoded.Visibility, payload.Visibility)
	}
	if decoded.Content != payload.Content {
		t.Errorf("Content = %q, want %q", decoded.Content, payload.Content)
	}
	if len(decoded.MediaURLs) != len(payload.MediaURLs) {
		t.Fatalf("MediaURLs len = %d, want %d", len(decoded.MediaURLs), len(payload.MediaURLs))
	}
	if decoded.MediaURLs[0] != payload.MediaURLs[0] {
		t.Errorf("MediaURLs[0] = %q, want %q", decoded.MediaURLs[0], payload.MediaURLs[0])
	}
}

func TestEventMetadataJSONRoundtrip(t *testing.T) {
	meta := EventMetadata{
		GDPRCompliant: true,
		UserConsent:   true,
		DataCategory:  DataCategoryMedia,
		CrossBorderOK: true,
	}

	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded EventMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.GDPRCompliant != meta.GDPRCompliant {
		t.Errorf("GDPRCompliant = %v, want %v", decoded.GDPRCompliant, meta.GDPRCompliant)
	}
	if decoded.UserConsent != meta.UserConsent {
		t.Errorf("UserConsent = %v, want %v", decoded.UserConsent, meta.UserConsent)
	}
	if decoded.DataCategory != meta.DataCategory {
		t.Errorf("DataCategory = %q, want %q", decoded.DataCategory, meta.DataCategory)
	}
	if decoded.CrossBorderOK != meta.CrossBorderOK {
		t.Errorf("CrossBorderOK = %v, want %v", decoded.CrossBorderOK, meta.CrossBorderOK)
	}
}
