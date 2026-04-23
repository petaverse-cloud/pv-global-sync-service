package handler

import (
	"encoding/json"
	"testing"
)

// TestUpsertUserRequest_JSONMarshaling verifies that the new fields are optional and parsed correctly.
// This ensures backwards compatibility for legacy payloads that do not include author metadata.
func TestUpsertUserRequest_JSONMarshaling(t *testing.T) {
	tests := []struct {
		name          string
		jsonBody      string
		wantEmailHash string
		wantRegion    string
		wantSlug      *int64
		wantNickname  string
	}{
		{
			name:          "Legacy payload (no author metadata)",
			jsonBody:      `{"emailHash":"hash1", "userId":1, "region":"sea"}`,
			wantEmailHash: "hash1",
			wantRegion:    "sea",
			wantSlug:      nil,
			wantNickname:  "",
		},
		{
			name:          "New payload (with author metadata)",
			jsonBody:      `{"emailHash":"hash2", "userId":2, "region":"eu", "authorSlug":123, "nickname":"Alice", "avatarUrl":"http://a.com/1.jpg"}`,
			wantEmailHash: "hash2",
			wantRegion:    "eu",
			wantSlug:      func() *int64 { v := int64(123); return &v }(),
			wantNickname:  "Alice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req UpsertUserRequest
			err := json.Unmarshal([]byte(tt.jsonBody), &req)
			if err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			if req.EmailHash != tt.wantEmailHash {
				t.Errorf("EmailHash = %v, want %v", req.EmailHash, tt.wantEmailHash)
			}
			if req.Region != tt.wantRegion {
				t.Errorf("Region = %v, want %v", req.Region, tt.wantRegion)
			}
			
			// Check pointer equality for slug
			if tt.wantSlug == nil && req.AuthorSlug != nil {
				t.Errorf("AuthorSlug should be nil, got %v", req.AuthorSlug)
			} else if tt.wantSlug != nil {
				if req.AuthorSlug == nil {
					t.Errorf("AuthorSlug should be %v, got nil", *tt.wantSlug)
				} else if *req.AuthorSlug != *tt.wantSlug {
					t.Errorf("AuthorSlug = %v, want %v", *req.AuthorSlug, *tt.wantSlug)
				}
			}
			
			if req.Nickname != tt.wantNickname {
				t.Errorf("Nickname = %v, want %v", req.Nickname, tt.wantNickname)
			}
		})
	}
}
