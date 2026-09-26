package library

import "testing"

func TestNormalizeLang(t *testing.T) {
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"en", "en", true},
		{"eng", "en", true},
		{"EN", "en", true},
		{" tr ", "tr", true},
		{"tur", "tr", true},
		{"ger", "de", true},
		{"deu", "de", true},
		{"fre", "fr", true},
		{"chi", "zh", true},
		{"cze", "cs", true},
		{"hi", "hi", true},
		{"pt", "pt", true},
		{"fil", "fil", true}, // no 2-letter code
		{"", "", false},
		{"q", "q", false},
		{"xx1", "xx1", false},
		{"english", "english", false},
		{"und", "und", false},
	}
	for _, tt := range tests {
		got, ok := NormalizeLang(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("NormalizeLang(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}
