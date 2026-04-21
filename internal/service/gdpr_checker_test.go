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

func TestGDPRChecker_Check_MediaAndEdgeCases(t *testing.T) {
	log, _ := logger.New("warn", "console")
	c := &GDPRChecker{log: log}

	tests := []struct {
		name   string
		event  *model.CrossRegionSyncEvent
		expect CheckResult
	}{
		{
			name: "TIER_4 Media with mediaUrls -> AllowedGlobal",
			event: &model.CrossRegionSyncEvent{
				EventID:   "evt_media_ok",
				EventType: model.EventTypePostCreated,
				Payload: model.EventPayload{
					PostID:     100,
					Visibility: model.VisibilityGlobal,
					MediaURLs:  []string{"https://cdn.example.com/img1.jpg"},
				},
				Metadata: model.EventMetadata{
					DataCategory: model.DataCategoryMedia,
					UserConsent:  true,
				},
			},
			expect: AllowedGlobal,
		},
		{
			name: "TIER_4 Media without mediaUrls -> DeniedMedia",
			event: &model.CrossRegionSyncEvent{
				EventID:   "evt_media_no_urls",
				EventType: model.EventTypePostCreated,
				Payload: model.EventPayload{
					PostID:     101,
					Visibility: model.VisibilityGlobal,
				},
				Metadata: model.EventMetadata{
					DataCategory: model.DataCategoryMedia,
					UserConsent:  true,
				},
			},
			expect: DeniedMedia,
		},
		{
			name: "empty visibility -> DeniedPrivate (default case)",
			event: &model.CrossRegionSyncEvent{
				EventID:   "evt_empty_vis",
				EventType: model.EventTypePostCreated,
				Payload: model.EventPayload{
					PostID:     102,
					Visibility: "",
				},
				Metadata: model.EventMetadata{
					DataCategory: model.DataCategoryUGC,
					UserConsent:  true,
				},
			},
			expect: DeniedPrivate,
		},
		{
			name: "unknown visibility -> DeniedPrivate (default case)",
			event: &model.CrossRegionSyncEvent{
				EventID:   "evt_unknown_vis",
				EventType: model.EventTypePostCreated,
				Payload: model.EventPayload{
					PostID:     103,
					Visibility: "UNKNOWN",
				},
				Metadata: model.EventMetadata{
					DataCategory: model.DataCategoryUGC,
					UserConsent:  true,
				},
			},
			expect: DeniedPrivate,
		},
		{
			name: "TIER_3 system + private visibility -> AllowedSystemData (system takes precedence)",
			event: &model.CrossRegionSyncEvent{
				EventID:   "evt_sys_private",
				EventType: model.EventTypePostCreated,
				Payload: model.EventPayload{
					PostID:     104,
					Visibility: model.VisibilityPrivate,
				},
				Metadata: model.EventMetadata{
					DataCategory: model.DataCategorySystem,
				},
			},
			expect: AllowedSystemData,
		},
		{
			name: "TIER_1 PII + global + consent -> DeniedPII (PII takes precedence)",
			event: &model.CrossRegionSyncEvent{
				EventID:   "evt_pii_global_consent",
				EventType: model.EventTypePostCreated,
				Payload: model.EventPayload{
					PostID:     105,
					Visibility: model.VisibilityGlobal,
				},
				Metadata: model.EventMetadata{
					DataCategory: model.DataCategoryPII,
					UserConsent:  true,
				},
			},
			expect: DeniedPII,
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
