package service

import (
	"testing"
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
