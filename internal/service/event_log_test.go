package service

import (
	"strings"
	"testing"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/model"
)

func TestParseEvent(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name:    "valid event",
			body:    `{"eventId":"evt1","eventType":"POST_CREATED","sourceRegion":"EU","targetRegion":"NA","timestamp":123,"payload":{"postId":1,"authorId":2,"authorRegion":"EU","visibility":"GLOBAL","content":"hello #world","mediaUrls":[]},"metadata":{"gdprCompliant":true,"userConsent":true,"dataCategory":"TIER_2","crossBorderOk":true}}`,
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			body:    `not json`,
			wantErr: true,
		},
		{
			name:    "missing eventId",
			body:    `{"eventType":"POST_CREATED","payload":{"postId":1}}`,
			wantErr: true,
		},
		{
			name:    "missing eventType",
			body:    `{"eventId":"evt1","payload":{"postId":1}}`,
			wantErr: true,
		},
		{
			name:    "missing postId",
			body:    `{"eventId":"evt1","eventType":"POST_CREATED","payload":{"authorId":2}}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := ParseEvent([]byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && event == nil {
				t.Error("ParseEvent() returned nil event without error")
			}
			if !tt.wantErr && event.EventID != "evt1" {
				t.Errorf("ParseEvent() eventID = %q, want %q", event.EventID, "evt1")
			}
		})
	}
}

func TestParseEvent_EmptyAndNullBodies(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
		errSub  string
	}{
		{
			name:    "empty body",
			body:    "",
			wantErr: true,
			errSub:  "parse sync event",
		},
		{
			name:    "whitespace only",
			body:    "   \t\n  ",
			wantErr: true,
			errSub:  "parse sync event",
		},
		{
			name:    "null body",
			body:    "null",
			wantErr: true,
			errSub:  "missing eventId",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := ParseEvent([]byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if event == nil {
					t.Fatal("expected non-nil event")
				}
				return
			}
			if err == nil {
				t.Fatal("expected error but got nil")
			}
			if tt.errSub != "" && !strings.Contains(err.Error(), tt.errSub) {
				t.Errorf("ParseEvent() error = %v, want substring %q", err, tt.errSub)
			}
		})
	}
}

func TestParseEvent_ExtraUnknownFields(t *testing.T) {
	body := `{
		"eventId": "evt-extra",
		"eventType": "POST_CREATED",
		"sourceRegion": "EU",
		"targetRegion": "NA",
		"timestamp": 999,
		"payload": {
			"postId": 42,
			"authorId": 7,
			"authorRegion": "EU",
			"visibility": "GLOBAL",
			"content": "test",
			"unknownPayloadField": "ignored"
		},
		"metadata": {
			"gdprCompliant": true,
			"userConsent": true,
			"dataCategory": "TIER_2",
			"crossBorderOk": true,
			"unknownMetaField": 12345
		},
		"topLevelUnknownField": "also ignored",
		"anotherUnknown": {"nested": true}
	}`

	event, err := ParseEvent([]byte(body))
	if err != nil {
		t.Fatalf("ParseEvent() unexpected error: %v", err)
	}
	if event == nil {
		t.Fatal("ParseEvent() returned nil event")
	}
	if event.EventID != "evt-extra" {
		t.Errorf("EventID = %q, want %q", event.EventID, "evt-extra")
	}
	if event.EventType != model.EventTypePostCreated {
		t.Errorf("EventType = %q, want %q", event.EventType, model.EventTypePostCreated)
	}
	if event.Payload.PostID != 42 {
		t.Errorf("PostID = %d, want 42", event.Payload.PostID)
	}
	if event.Timestamp != 999 {
		t.Errorf("Timestamp = %d, want 999", event.Timestamp)
	}
}

func TestParseEvent_ZeroPostID(t *testing.T) {
	body := `{"eventId":"evt1","eventType":"POST_CREATED","payload":{"postId":0,"authorId":1}}`
	event, err := ParseEvent([]byte(body))
	if err == nil {
		t.Fatal("ParseEvent() expected error for postId=0, got nil")
	}
	if event != nil {
		t.Error("ParseEvent() expected nil event on error")
	}
	if !strings.Contains(err.Error(), "missing postId") {
		t.Errorf("ParseEvent() error = %v, want substring %q", err, "missing postId")
	}
}

func TestParseEvent_AllEventTypes(t *testing.T) {
	eventTypes := []struct {
		name  string
		value model.SyncEventType
	}{
		{"POST_CREATED", model.EventTypePostCreated},
		{"POST_UPDATED", model.EventTypePostUpdated},
		{"POST_DELETED", model.EventTypePostDeleted},
		{"POST_STATS_UPDATED", model.EventTypePostStatsUpdated},
	}

	for _, et := range eventTypes {
		t.Run(et.name, func(t *testing.T) {
			body := `{"eventId":"evt-type","eventType":"` + string(et.value) + `","payload":{"postId":1}}`
			event, err := ParseEvent([]byte(body))
			if err != nil {
				t.Fatalf("ParseEvent() unexpected error: %v", err)
			}
			if event == nil {
				t.Fatal("expected non-nil event")
			}
			if event.EventType != et.value {
				t.Errorf("EventType = %q, want %q", event.EventType, et.value)
			}
		})
	}
}

func TestParseEvent_FieldVerification(t *testing.T) {
	body := `{
		"eventId": "evt-verify",
		"eventType": "POST_UPDATED",
		"sourceRegion": "NA",
		"targetRegion": "EU",
		"timestamp": 1700000000,
		"payload": {
			"postId": 9876,
			"authorId": 5432,
			"authorRegion": "NA",
			"visibility": "FOLLOWERS",
			"content": "updated content here"
		},
		"metadata": {
			"gdprCompliant": true,
			"userConsent": false,
			"dataCategory": "TIER_1",
			"crossBorderOk": false
		}
	}`

	event, err := ParseEvent([]byte(body))
	if err != nil {
		t.Fatalf("ParseEvent() unexpected error: %v", err)
	}
	if event == nil {
		t.Fatal("expected non-nil event")
	}

	// Verify top-level fields
	if event.EventID != "evt-verify" {
		t.Errorf("EventID = %q, want %q", event.EventID, "evt-verify")
	}
	if event.EventType != model.EventTypePostUpdated {
		t.Errorf("EventType = %q, want %q", event.EventType, model.EventTypePostUpdated)
	}
	if event.SourceRegion != model.RegionNA {
		t.Errorf("SourceRegion = %q, want %q", event.SourceRegion, model.RegionNA)
	}
	if event.TargetRegion != model.RegionEU {
		t.Errorf("TargetRegion = %q, want %q", event.TargetRegion, model.RegionEU)
	}
	if event.Timestamp != 1700000000 {
		t.Errorf("Timestamp = %d, want 1700000000", event.Timestamp)
	}

	// Verify payload fields
	if event.Payload.PostID != 9876 {
		t.Errorf("Payload.PostID = %d, want 9876", event.Payload.PostID)
	}
	if event.Payload.AuthorID != 5432 {
		t.Errorf("Payload.AuthorID = %d, want 5432", event.Payload.AuthorID)
	}
	if event.Payload.AuthorRegion != model.RegionNA {
		t.Errorf("Payload.AuthorRegion = %q, want %q", event.Payload.AuthorRegion, model.RegionNA)
	}
	if event.Payload.Visibility != model.VisibilityFollowers {
		t.Errorf("Payload.Visibility = %q, want %q", event.Payload.Visibility, model.VisibilityFollowers)
	}
	if event.Payload.Content != "updated content here" {
		t.Errorf("Payload.Content = %q, want %q", event.Payload.Content, "updated content here")
	}

	// Verify metadata fields
	if !event.Metadata.GDPRCompliant {
		t.Error("Metadata.GDPRCompliant = false, want true")
	}
	if event.Metadata.UserConsent {
		t.Error("Metadata.UserConsent = true, want false")
	}
	if event.Metadata.DataCategory != model.DataCategoryPII {
		t.Errorf("Metadata.DataCategory = %q, want %q", event.Metadata.DataCategory, model.DataCategoryPII)
	}
	if event.Metadata.CrossBorderOK {
		t.Error("Metadata.CrossBorderOK = true, want false")
	}
}

func TestParseEvent_MediaURLs(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantMediaURLs []string
		wantMediaLen  int
	}{
		{
			name: "multiple media URLs",
			body: `{
				"eventId": "evt-media",
				"eventType": "POST_CREATED",
				"payload": {
					"postId": 1,
					"mediaUrls": ["https://cdn.example.com/img1.jpg", "https://cdn.example.com/vid1.mp4", "https://cdn.example.com/img2.png"]
				}
			}`,
			wantMediaURLs: []string{
				"https://cdn.example.com/img1.jpg",
				"https://cdn.example.com/vid1.mp4",
				"https://cdn.example.com/img2.png",
			},
			wantMediaLen: 3,
		},
		{
			name: "single media URL",
			body: `{
				"eventId": "evt-media2",
				"eventType": "POST_CREATED",
				"payload": {
					"postId": 2,
					"mediaUrls": ["https://cdn.example.com/only.jpg"]
				}
			}`,
			wantMediaURLs: []string{"https://cdn.example.com/only.jpg"},
			wantMediaLen:  1,
		},
		{
			name: "empty media URLs array",
			body: `{
				"eventId": "evt-media3",
				"eventType": "POST_CREATED",
				"payload": {
					"postId": 3,
					"mediaUrls": []
				}
			}`,
			wantMediaURLs: nil,
			wantMediaLen:  0,
		},
		{
			name: "no mediaUrls field",
			body: `{
				"eventId": "evt-media4",
				"eventType": "POST_CREATED",
				"payload": {
					"postId": 4
				}
			}`,
			wantMediaURLs: nil,
			wantMediaLen:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := ParseEvent([]byte(tt.body))
			if err != nil {
				t.Fatalf("ParseEvent() unexpected error: %v", err)
			}
			if event == nil {
				t.Fatal("expected non-nil event")
			}

			if len(event.Payload.MediaURLs) != tt.wantMediaLen {
				t.Errorf("len(MediaURLs) = %d, want %d", len(event.Payload.MediaURLs), tt.wantMediaLen)
			}

			for i, url := range tt.wantMediaURLs {
				if i >= len(event.Payload.MediaURLs) {
					t.Errorf("MediaURLs[%d] missing, want %q", i, url)
					continue
				}
				if event.Payload.MediaURLs[i] != url {
					t.Errorf("MediaURLs[%d] = %q, want %q", i, event.Payload.MediaURLs[i], url)
				}
			}
		})
	}
}

func TestParseEvent_NestedEmptyPayload(t *testing.T) {
	body := `{"eventId":"evt-nested","eventType":"POST_DELETED","payload":{"postId":100,"content":"","mediaUrls":[]}}`
	event, err := ParseEvent([]byte(body))
	if err != nil {
		t.Fatalf("ParseEvent() unexpected error: %v", err)
	}
	if event == nil {
		t.Fatal("expected non-nil event")
	}
	if event.Payload.PostID != 100 {
		t.Errorf("PostID = %d, want 100", event.Payload.PostID)
	}
	if event.Payload.Content != "" {
		t.Errorf("Content = %q, want empty", event.Payload.Content)
	}
	if len(event.Payload.MediaURLs) != 0 {
		t.Errorf("len(MediaURLs) = %d, want 0", len(event.Payload.MediaURLs))
	}
}

func TestParseEvent_WhitespaceAndZeroValues(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		errSub string
	}{
		{
			name:   "empty string eventId",
			body:   `{"eventId":"","eventType":"POST_CREATED","payload":{"postId":1}}`,
			errSub: "missing eventId",
		},
		{
			name:   "empty string eventType",
			body:   `{"eventId":"evt1","eventType":"","payload":{"postId":1}}`,
			errSub: "missing eventType",
		},
		{
			name:   "missing payload entirely",
			body:   `{"eventId":"evt1","eventType":"POST_CREATED"}`,
			errSub: "missing postId",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := ParseEvent([]byte(tt.body))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if event != nil {
				t.Error("expected nil event on error")
			}
			if !strings.Contains(err.Error(), tt.errSub) {
				t.Errorf("error = %v, want substring %q", err, tt.errSub)
			}
		})
	}
}
