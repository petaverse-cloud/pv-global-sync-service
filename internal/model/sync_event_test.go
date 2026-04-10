package model

import (
	"encoding/json"
	"testing"
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
