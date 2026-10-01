package api

import "testing"

func TestGuestURL(t *testing.T) {
	tests := []struct {
		bind, port, want string
	}{
		{"100.101.102.103", "8420", "http://100.101.102.103:8420"},
		{"[fd7a:115c:a1e0::1]", "8420", "http://[fd7a:115c:a1e0::1]:8420"}, // as compose's ports need it
		{"fd7a:115c:a1e0::1", "8420", "http://[fd7a:115c:a1e0::1]:8420"},
		{"[::1]", "8420", ""},
		{"127.0.0.1", "8420", ""}, // the default: guests can't reach it yet
		{"::1", "8420", ""},
		{"0.0.0.0", "8420", ""}, // every address, so no one to give out
		{"", "8420", ""},
		{"my-host", "8420", ""}, // compose binds IPs only
		{"100.101.102.103", "", ""},
	}
	for _, tt := range tests {
		if got := GuestURL(tt.bind, tt.port); got != tt.want {
			t.Errorf("GuestURL(%q, %q) = %q, want %q", tt.bind, tt.port, got, tt.want)
		}
	}
}
