package service

import (
	"reflect"
	"testing"
)

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
		{
			name:    "empty string",
			content: "",
			want:    nil,
		},
		{
			name:    "no tags",
			content: "hello world, this is a plain post",
			want:    nil,
		},
		{
			name:    "single tag",
			content: "check out #GoLang",
			want:    []string{"GoLang"},
		},
		{
			name:    "multiple tags scattered",
			content: "#hello world #world",
			want:    []string{"hello", "world"},
		},
		{
			name:    "consecutive tags",
			content: "#a#b#c",
			want:    []string{"a", "b", "c"},
		},
		{
			name:    "tag with underscores",
			content: "love #open_source",
			want:    []string{"open_source"},
		},
		{
			name:    "tag with numbers",
			content: "#go123 is great",
			want:    []string{"go123"},
		},
		{
			name:    "duplicate tags removed",
			content: "#golang and #golang again",
			want:    []string{"golang"},
		},
		{
			name:    "tag at start of string",
			content: "#trending now",
			want:    []string{"trending"},
		},
		{
			name:    "tag at end of string",
			content: "check this #viral",
			want:    []string{"viral"},
		},
		{
			name:    "hash followed by space only",
			content: "use # hashtag not like that",
			want:    nil,
		},
		{
			name:    "hash at end with no following char",
			content: "trailing #",
			want:    nil,
		},
		{
			name:    "unicode content with ascii tags",
			content: "\u4f60\u597d #\u4e2d\u6587 #English \u4e16\u754c",
			want:    []string{"English"},
		},
		{
			name:    "tag separated by punctuation",
			content: "love #cats! and #dogs.",
			want:    []string{"cats", "dogs"},
		},
		{
			name:    "mixed valid and invalid after hash",
			content: "#valid #not-valid #_also_valid",
			want:    []string{"valid", "not", "_also_valid"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractHashtags(tt.content)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("extractHashtags(%q) = %v, want %v", tt.content, got, tt.want)
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
		{
			name:    "empty string",
			content: "",
			maxLen:  10,
			want:    "",
		},
		{
			name:    "shorter than max",
			content: "hi",
			maxLen:  10,
			want:    "hi",
		},
		{
			name:    "exact length",
			content: "hello",
			maxLen:  5,
			want:    "hello",
		},
		{
			name:    "just over max length",
			content: "hello!",
			maxLen:  5,
			want:    "hello...",
		},
		{
			name:    "very long string truncated",
			content: "abcdefghijklmnopqrstuvwxyz",
			maxLen:  10,
			want:    "abcdefghij...",
		},
		{
			name:    "maxLen zero",
			content: "hello",
			maxLen:  0,
			want:    "...",
		},
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
		// Valid: lowercase
		{"lowercase a", 'a', true},
		{"lowercase m", 'm', true},
		{"lowercase z", 'z', true},
		// Valid: uppercase
		{"uppercase A", 'A', true},
		{"uppercase M", 'M', true},
		{"uppercase Z", 'Z', true},
		// Valid: digits
		{"digit 0", '0', true},
		{"digit 5", '5', true},
		{"digit 9", '9', true},
		// Valid: underscore
		{"underscore", '_', true},
		// Invalid: space
		{"space", ' ', false},
		// Invalid: hyphen
		{"hyphen", '-', false},
		// Invalid: exclamation
		{"exclamation", '!', false},
		// Invalid: at sign
		{"at sign", '@', false},
		// Invalid: hash
		{"hash", '#', false},
		// Invalid: period
		{"period", '.', false},
		// Invalid: tab
		{"tab", '\t', false},
		// Invalid: newline
		{"newline", '\n', false},
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
