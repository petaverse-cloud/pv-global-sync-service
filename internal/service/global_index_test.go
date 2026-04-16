package service

import (
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
