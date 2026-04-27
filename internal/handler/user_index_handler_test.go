package handler

import (
	"encoding/json"
	"testing"
)

// TestUpsertUserRequest_JSONMarshaling verifies that the uid-based payload is parsed correctly.
func TestUpsertUserRequest_JSONMarshaling(t *testing.T) {
	tests := []struct {
		name          string
		jsonBody      string
		wantUID       int64
		wantRegion    string
		wantEmailHash *string
	}{
		{
			name:          "Payload with emailHash",
			jsonBody:      `{"uid": 2206208000, "region": "sea", "emailHash": "e928de69..."}`,
			wantUID:       2206208000,
			wantRegion:    "sea",
			wantEmailHash: func() *string { v := "e928de69..."; return &v }(),
		},
		{
			name:          "Payload without emailHash (OAuth user)",
			jsonBody:      `{"uid": 4198502400, "region": "eu"}`,
			wantUID:       4198502400,
			wantRegion:    "eu",
			wantEmailHash: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req UpsertUserRequest
			err := json.Unmarshal([]byte(tt.jsonBody), &req)
			if err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			if req.UID != tt.wantUID {
				t.Errorf("UID = %v, want %v", req.UID, tt.wantUID)
			}
			if req.Region != tt.wantRegion {
				t.Errorf("Region = %v, want %v", req.Region, tt.wantRegion)
			}
			if tt.wantEmailHash == nil && req.EmailHash != nil {
				t.Errorf("EmailHash should be nil, got %v", *req.EmailHash)
			} else if tt.wantEmailHash != nil {
				if req.EmailHash == nil {
					t.Errorf("EmailHash should be %v, got nil", *tt.wantEmailHash)
				} else if *req.EmailHash != *tt.wantEmailHash {
					t.Errorf("EmailHash = %v, want %v", *req.EmailHash, *tt.wantEmailHash)
				}
			}
		})
	}
}
