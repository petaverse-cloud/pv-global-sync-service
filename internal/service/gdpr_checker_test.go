package service

import (
	"testing"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/model"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

func TestGDPRChecker_Check(t *testing.T) {
	// Create checker with a real logger (we only test logic, not DB/Redis calls)
	log, _ := logger.New("warn", "console")
	c := &GDPRChecker{log: log}

	tests := []struct {
		name   string
		event  *model.CrossRegionSyncEvent
		expect CheckResult
	}{
		{
			name: "deny PII data",
			event: &model.CrossRegionSyncEvent{
				EventID:   "evt1",
				EventType: model.EventTypePostCreated,
				Payload:   model.EventPayload{PostID: 1, Visibility: model.VisibilityGlobal},
				Metadata:  model.EventMetadata{DataCategory: model.DataCategoryPII, UserConsent: true},
			},
			expect: DeniedPII,
		},
		{
			name: "allow system data",
			event: &model.CrossRegionSyncEvent{
				EventID:   "evt2",
				EventType: model.EventTypePostCreated,
				Payload:   model.EventPayload{PostID: 2, Visibility: model.VisibilityGlobal},
				Metadata:  model.EventMetadata{DataCategory: model.DataCategorySystem},
			},
			expect: AllowedSystemData,
		},
		{
			name: "deny private content",
			event: &model.CrossRegionSyncEvent{
				EventID:   "evt3",
				EventType: model.EventTypePostCreated,
				Payload:   model.EventPayload{PostID: 3, Visibility: model.VisibilityPrivate},
				Metadata:  model.EventMetadata{DataCategory: model.DataCategoryUGC, UserConsent: true},
			},
			expect: DeniedPrivate,
		},
		{
			name: "deny followers-only content",
			event: &model.CrossRegionSyncEvent{
				EventID:   "evt4",
				EventType: model.EventTypePostCreated,
				Payload:   model.EventPayload{PostID: 4, Visibility: model.VisibilityFollowers},
				Metadata:  model.EventMetadata{DataCategory: model.DataCategoryUGC, UserConsent: true},
			},
			expect: DeniedFollowers,
		},
		{
			name: "deny regional content",
			event: &model.CrossRegionSyncEvent{
				EventID:   "evt5",
				EventType: model.EventTypePostCreated,
				Payload:   model.EventPayload{PostID: 5, Visibility: model.VisibilityRegional},
				Metadata:  model.EventMetadata{DataCategory: model.DataCategoryUGC, UserConsent: true},
			},
			expect: DeniedRegional,
		},
		{
			name: "deny global without consent",
			event: &model.CrossRegionSyncEvent{
				EventID:   "evt6",
				EventType: model.EventTypePostCreated,
				Payload:   model.EventPayload{PostID: 6, Visibility: model.VisibilityGlobal},
				Metadata:  model.EventMetadata{DataCategory: model.DataCategoryUGC, UserConsent: false},
			},
			expect: DeniedNoConsent,
		},
		{
			name: "allow global with consent",
			event: &model.CrossRegionSyncEvent{
				EventID:   "evt7",
				EventType: model.EventTypePostCreated,
				Payload:   model.EventPayload{PostID: 7, Visibility: model.VisibilityGlobal},
				Metadata:  model.EventMetadata{DataCategory: model.DataCategoryUGC, UserConsent: true},
			},
			expect: AllowedGlobal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Check(tt.event)
			if result.Allowed != tt.expect.Allowed {
				t.Errorf("Check() allowed = %v, want %v", result.Allowed, tt.expect.Allowed)
			}
			if result.Reason != tt.expect.Reason {
				t.Errorf("Check() reason = %q, want %q", result.Reason, tt.expect.Reason)
			}
		})
	}
}

func TestExtractHashtags(t *testing.T) {
	tests := []struct {
		content string
		want    []string
	}{
		{"", nil},
		{"no tags here", nil},
		{"Hello #world", []string{"world"}},
		{"#tag1 and #tag2", []string{"tag1", "tag2"}},
		{"#dup and #dup", []string{"dup"}},
		{"#hello_world", []string{"hello_world"}},
		{"#test at the end", []string{"test"}},
		{"at the end #test", []string{"test"}},
		{"#a#b#c", []string{"a", "b", "c"}},
		{"# with space after", []string{}},
	}

	for _, tt := range tests {
		got := extractHashtags(tt.content)
		if len(got) != len(tt.want) {
			t.Errorf("extractHashtags(%q) len = %d, want %d, got %v", tt.content, len(got), len(tt.want), got)
			continue
		}
		for i := range tt.want {
			if got[i] != tt.want[i] {
				t.Errorf("extractHashtags(%q)[%d] = %q, want %q", tt.content, i, got[i], tt.want[i])
			}
		}
	}
}

func TestTruncatePreview(t *testing.T) {
	tests := []struct {
		content string
		maxLen  int
		want    string
	}{
		{"short", 10, "short"},
		{"exactly10", 10, "exactly10"},
		{"this is a longer text that should be truncated", 20, "this is a longer tex..."},
	}

	for _, tt := range tests {
		got := truncatePreview(tt.content, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncatePreview(%q, %d) = %q, want %q", tt.content, tt.maxLen, got, tt.want)
		}
	}
}
