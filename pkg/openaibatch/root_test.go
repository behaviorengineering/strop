package openaibatch

import (
	"testing"
)

func TestGatewayRoot(t *testing.T) {
	tests := []struct {
		in   string
		want string
		err  bool
	}{
		{"https://api.example.com", "https://api.example.com", false},
		{"https://api.example.com/", "https://api.example.com", false},
		{"https://api.example.com/v1", "https://api.example.com", false},
		{"https://api.example.com/v1/", "https://api.example.com", false},
		{"https://api.example.com/v1/chat/completions", "https://api.example.com", false},
		{"", "", true},
		{"   ", "", true},
	}
	for _, tc := range tests {
		got, err := GatewayRoot(tc.in)
		if tc.err {
			if err == nil {
				t.Fatalf("GatewayRoot(%q): expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("GatewayRoot(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("GatewayRoot(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
