package web

import (
	"testing"
	"testing/fstest"
)

func TestBuildID(t *testing.T) {
	tests := []struct {
		name  string
		build fstest.MapFS
		want  string
	}{
		{"version file", fstest.MapFS{"_app/version.json": {Data: []byte(`{"version":"1727000000000"}`)}}, "1727000000000"},
		{"no web build yet", fstest.MapFS{".gitkeep": {}}, ""},
		{"broken file", fstest.MapFS{"_app/version.json": {Data: []byte(`{`)}}, ""},
	}
	for _, tt := range tests {
		if got := BuildID(tt.build); got != tt.want {
			t.Errorf("%s: BuildID = %q, want %q", tt.name, got, tt.want)
		}
	}
}
