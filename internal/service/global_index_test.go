package service

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/model"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

func makeEvent(eventType model.SyncEventType, postUid int64, authorUid int64, content string) *model.CrossRegionSyncEvent {
	return &model.CrossRegionSyncEvent{
		EventID:      "evt_test_001",
		EventType:    eventType,
		SourceRegion: model.RegionSEA,
		TargetRegion: model.RegionEU,
		Timestamp:    time.Now().Unix(),
		Payload: model.EventPayload{
			PostUid:      postUid,
			AuthorUid:    authorUid,
			AuthorRegion: model.RegionSEA,
			Visibility:   model.VisibilityGlobal,
			Content:      content,
			MediaURLs:    []string{"https://cdn.example.com/img.jpg"},
		},
		Metadata: model.EventMetadata{
			GDPRCompliant: true,
			UserConsent:   true,
			DataCategory:  model.DataCategoryUGC,
			CrossBorderOK: true,
		},
	}
}

func TestPgtypeArray_Scan(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    []string
		wantErr bool
	}{
		{"nil", nil, nil, false},
		{"empty array", []byte("{}"), []string{}, false},
		{"single element", []byte("{hello}"), []string{"hello"}, false},
		{"multiple elements", []byte("{hello,world}"), []string{"hello", "world"}, false},
		{"quoted elements", []byte(`{"hello world","foo bar"}`), []string{"hello world", "foo bar"}, false},
		{"urls", []byte(`{https://a.com/1,https://b.com/2}`), []string{"https://a.com/1", "https://b.com/2"}, false},
		{"invalid type", 123, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var arr pgtypeArray
			err := arr.Scan(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Scan() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(arr) != len(tt.want) {
					t.Errorf("Scan() len = %d, want %d", len(arr), len(tt.want))
					return
				}
				for i := range tt.want {
					if arr[i] != tt.want[i] {
						t.Errorf("Scan()[%d] = %q, want %q", i, arr[i], tt.want[i])
					}
				}
			}
		})
	}
}

func TestParseTextArrayHelper(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty string", "", []string{""}},
		{"empty array", "{}", []string{}},
		{"single", "{hello}", []string{"hello"}},
		{"multiple", "{a,b,c}", []string{"a", "b", "c"}},
		{"quoted", `{"hello world"}`, []string{"hello world"}},
		{"urls", `{https://a.com/1,https://b.com/2}`, []string{"https://a.com/1", "https://b.com/2"}},
		{"not array format", "just text", []string{"just text"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTextArray(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("parseTextArray(%q) len = %d, want %d, got %v", tt.input, len(got), len(tt.want), got)
				return
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("parseTextArray(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestExtractHashtags(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{"empty string", "", nil},
		{"no tags", "hello world, this is a plain post", nil},
		{"single tag", "check out #GoLang", []string{"GoLang"}},
		{"multiple tags scattered", "#hello world #world", []string{"hello", "world"}},
		{"consecutive tags", "#a#b#c", []string{"a", "b", "c"}},
		{"tag with underscores", "love #open_source", []string{"open_source"}},
		{"tag with numbers", "#go123 is great", []string{"go123"}},
		{"duplicate tags removed", "#golang and #golang again", []string{"golang"}},
		{"tag at start of string", "#trending now", []string{"trending"}},
		{"tag at end of string", "check this #viral", []string{"viral"}},
		{"hash followed by space only", "use # hashtag not like that", nil},
		{"hash at end with no following char", "trailing #", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractHashtags(tt.content)
			if len(got) != len(tt.want) {
				t.Errorf("extractHashtags(%q) len=%d want=%d got=%v", tt.content, len(got), len(tt.want), got)
				return
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("extractHashtags(%q)[%d]=%q want=%q", tt.content, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestTruncatePreview(t *testing.T) {
	tests := []struct {
		name    string
		content string
		maxLen  int
		want    string
	}{
		{"empty string", "", 10, ""},
		{"shorter than max", "hi", 10, "hi"},
		{"exact length", "hello", 5, "hello"},
		{"just over max length", "hello!", 5, "hello..."},
		{"very long string truncated", "abcdefghijklmnopqrstuvwxyz", 10, "abcdefghij..."},
		{"maxLen zero", "hello", 0, "..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncatePreview(tt.content, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncatePreview(%q, %d) = %q, want %q", tt.content, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestIsTagChar(t *testing.T) {
	tests := []struct {
		name  string
		input byte
		want  bool
	}{
		{"lowercase a", 'a', true},
		{"lowercase z", 'z', true},
		{"uppercase A", 'A', true},
		{"uppercase Z", 'Z', true},
		{"digit 0", '0', true},
		{"digit 9", '9', true},
		{"underscore", '_', true},
		{"space", ' ', false},
		{"hyphen", '-', false},
		{"exclamation", '!', false},
		{"period", '.', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTagChar(tt.input)
			if got != tt.want {
				t.Errorf("isTagChar(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// ============================================
// GlobalIndexService CRUD tests
// ============================================

func TestInsertPost_NewPost(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	svc := NewGlobalIndexServiceWithDB(mock, logger.NewNop())
	event := makeEvent(model.EventTypePostCreated, 9000000001, 8000000001, "Hello world #test")
	mock.ExpectExec("INSERT INTO global_post_index").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := svc.InsertPost(context.Background(), event); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}
}

func TestInsertPost_WithAuthorProfile(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	svc := NewGlobalIndexServiceWithDB(mock, logger.NewNop())
	event := makeEvent(model.EventTypePostCreated, 9000000002, 8000000002, "Post with author")
	event.Payload.AuthorProfile = &model.AuthorProfile{Uid: 8000000002, Nickname: "TestAuthor", AvatarURL: "https://cdn.example.com/avatar.jpg"}
	mock.ExpectExec("INSERT INTO global_post_index").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := svc.InsertPost(context.Background(), event); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}
}

func TestInsertPost_ContentTruncated(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	svc := NewGlobalIndexServiceWithDB(mock, logger.NewNop())
	longContent := ""
	for i := 0; i < 600; i++ {
		longContent += "x"
	}
	event := makeEvent(model.EventTypePostCreated, 9000000004, 8000000004, longContent)
	mock.ExpectExec("INSERT INTO global_post_index").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := svc.InsertPost(context.Background(), event); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}
}

func TestUpdatePost_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	svc := NewGlobalIndexServiceWithDB(mock, logger.NewNop())
	event := makeEvent(model.EventTypePostUpdated, 9000000010, 8000000010, "Updated hello #updated")
	event.Payload.MediaURLs = []string{"https://cdn.example.com/new.jpg"}

	mock.ExpectExec("UPDATE global_post_index").
		WithArgs(
			"Updated hello #updated",
			pgxmock.AnyArg(),
			[]string{"updated"},
			event.Payload.MediaURLs,
			event.Payload.PostUid,
		).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err = svc.UpdatePost(context.Background(), event)
	if err != nil {
		t.Fatalf("UpdatePost failed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUpdatePost_NotFoundFallbackToInsert(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	svc := NewGlobalIndexServiceWithDB(mock, logger.NewNop())
	event := makeEvent(model.EventTypePostUpdated, 9000000099, 8000000099, "Fallback insert")

	mock.ExpectExec("UPDATE global_post_index").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), event.Payload.PostUid).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	mock.ExpectExec("INSERT INTO global_post_index").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
			pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = svc.UpdatePost(context.Background(), event)
	if err != nil {
		t.Fatalf("UpdatePost fallback failed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestDeletePost_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	svc := NewGlobalIndexServiceWithDB(mock, logger.NewNop())
	event := makeEvent(model.EventTypePostDeleted, 9000000011, 8000000011, "")

	mock.ExpectExec("DELETE FROM global_post_index").
		WithArgs(event.Payload.PostUid).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	err = svc.DeletePost(context.Background(), event)
	if err != nil {
		t.Fatalf("DeletePost failed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestDeletePost_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	svc := NewGlobalIndexServiceWithDB(mock, logger.NewNop())
	event := makeEvent(model.EventTypePostDeleted, 9999999999, 8000000012, "")

	mock.ExpectExec("DELETE FROM global_post_index").
		WithArgs(event.Payload.PostUid).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	err = svc.DeletePost(context.Background(), event)
	if err != nil {
		t.Fatalf("DeletePost should not error for not found: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUpdateStats_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	svc := NewGlobalIndexServiceWithDB(mock, logger.NewNop())

	mock.ExpectExec("UPDATE global_post_index").
		WithArgs(42, 7, 3, 150, int64(9000000020)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err = svc.UpdateStats(context.Background(), 9000000020, 42, 7, 3, 150)
	if err != nil {
		t.Fatalf("UpdateStats failed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestGetPost_Found(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	svc := NewGlobalIndexServiceWithDB(mock, logger.NewNop())
	now := time.Now().UTC()

	rows := pgxmock.NewRows([]string{
		"post_slug", "author_uid", "author_region", "content_preview", "visibility",
		"hashtags", "mentions", "media_urls_str",
		"likes_count", "comments_count", "shares_count", "views_count",
		"gdpr_compliant", "user_consent", "data_category", "created_at", "synced_at",
		"author_nickname", "author_avatar_url",
	}).AddRow(
		int64(9000000030), int64(8000000030), "SEA", "Hello world", "GLOBAL",
		[]byte("{test,go}"), []byte("{1,2}"), "https://a.jpg,https://b.jpg",
		10, 5, 2, 100,
		true, true, "TIER_2", now, now,
		nil, nil,
	)

	mock.ExpectQuery("SELECT").WithArgs(int64(9000000030)).WillReturnRows(rows)

	post, err := svc.GetPost(context.Background(), 9000000030)
	if err != nil {
		t.Fatalf("GetPost failed: %v", err)
	}
	if post == nil {
		t.Fatal("expected post, got nil")
	}
	if post.PostUid != 9000000030 {
		t.Errorf("PostUid = %d, want 9000000030", post.PostUid)
	}
	if post.AuthorUid != 8000000030 {
		t.Errorf("AuthorUid = %d, want 8000000030", post.AuthorUid)
	}
	if len(post.MediaURLs) != 2 {
		t.Errorf("MediaURLs len = %d, want 2", len(post.MediaURLs))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestGetPost_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	svc := NewGlobalIndexServiceWithDB(mock, logger.NewNop())

	mock.ExpectQuery("SELECT").WithArgs(int64(9999999999)).WillReturnError(pgx.ErrNoRows)

	post, err := svc.GetPost(context.Background(), 9999999999)
	if err != nil {
		t.Fatalf("GetPost should not error for not found: %v", err)
	}
	if post != nil {
		t.Errorf("expected nil post, got %+v", post)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUpsertUserIndex_Insert(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	svc := NewGlobalIndexServiceWithDB(mock, logger.NewNop())
	emailHash := "abc123def456"

	mock.ExpectExec("INSERT INTO users_global_index").
		WithArgs(int64(9000000100), "SEA", &emailHash).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = svc.UpsertUserIndex(context.Background(), 9000000100, "SEA", &emailHash)
	if err != nil {
		t.Fatalf("UpsertUserIndex failed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUpsertUserIndex_NilEmailHash(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	svc := NewGlobalIndexServiceWithDB(mock, logger.NewNop())

	mock.ExpectExec("INSERT INTO users_global_index").
		WithArgs(int64(9000000101), "EU", (*string)(nil)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err = svc.UpsertUserIndex(context.Background(), 9000000101, "EU", nil)
	if err != nil {
		t.Fatalf("UpsertUserIndex with nil emailHash failed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestFindRegionByEmailHash_Found(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	svc := NewGlobalIndexServiceWithDB(mock, logger.NewNop())

	rows := pgxmock.NewRows([]string{"region"}).AddRow("SEA")
	mock.ExpectQuery("SELECT region FROM users_global_index").WithArgs("hash123").WillReturnRows(rows)

	region, err := svc.FindRegionByEmailHash(context.Background(), "hash123")
	if err != nil {
		t.Fatalf("FindRegionByEmailHash failed: %v", err)
	}
	if region != "SEA" {
		t.Errorf("region = %q, want SEA", region)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestFindRegionByEmailHash_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	svc := NewGlobalIndexServiceWithDB(mock, logger.NewNop())

	mock.ExpectQuery("SELECT").WithArgs("nohash").WillReturnError(pgx.ErrNoRows)

	region, err := svc.FindRegionByEmailHash(context.Background(), "nohash")
	if err != nil {
		t.Fatalf("FindRegionByEmailHash should not error: %v", err)
	}
	if region != "" {
		t.Errorf("region = %q, want empty", region)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestFindRegionByUID_Found(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	svc := NewGlobalIndexServiceWithDB(mock, logger.NewNop())

	rows := pgxmock.NewRows([]string{"region"}).AddRow("EU")
	mock.ExpectQuery("SELECT region FROM users_global_index").WithArgs(int64(9000000200)).WillReturnRows(rows)

	region, err := svc.FindRegionByUID(context.Background(), 9000000200)
	if err != nil {
		t.Fatalf("FindRegionByUID failed: %v", err)
	}
	if region != "EU" {
		t.Errorf("region = %q, want EU", region)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestGetAllUserIndexEntries(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	svc := NewGlobalIndexServiceWithDB(mock, logger.NewNop())
	emailHash1 := "hash1"
	emailHash2 := "hash2"

	rows := pgxmock.NewRows([]string{"uid", "email_hash", "region"}).
		AddRow(int64(1), &emailHash1, "SEA").
		AddRow(int64(2), &emailHash2, "EU")

	mock.ExpectQuery("SELECT uid, email_hash, region FROM users_global_index").WillReturnRows(rows)

	entries, err := svc.GetAllUserIndexEntries(context.Background())
	if err != nil {
		t.Fatalf("GetAllUserIndexEntries failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len = %d, want 2", len(entries))
	}
	if entries[0].UID != 1 || entries[0].Region != "SEA" {
		t.Errorf("entry[0]: uid=%d region=%s", entries[0].UID, entries[0].Region)
	}
	if entries[1].UID != 2 || entries[1].Region != "EU" {
		t.Errorf("entry[1]: uid=%d region=%s", entries[1].UID, entries[1].Region)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
