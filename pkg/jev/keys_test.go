package jev

import (
	"testing"
)

func TestJoinSplit_roundTrip(t *testing.T) {
	cases := []struct {
		row, opt string
	}{
		{"hash1", "use:notification"},
		{"row__with__underscores", "skip"},
		{"unicode-日本", "use:newsletter"},
		{"a\x1eb", "opt:with:colons"},
	}
	for _, tc := range cases {
		key := JoinKey(tc.row, tc.opt)
		row, opt, err := SplitKey(key)
		if err != nil {
			t.Fatalf("SplitKey(%q): %v", key, err)
		}
		if row != tc.row || opt != tc.opt {
			t.Fatalf("round trip: got row=%q opt=%q want row=%q opt=%q", row, opt, tc.row, tc.opt)
		}
	}
}

func TestSplitKey_rejectsEmpty(t *testing.T) {
	if _, _, err := SplitKey(""); err == nil {
		t.Fatal("expected error for empty key")
	}
	if _, _, err := SplitKey(JoinKey("", "x")); err == nil {
		t.Fatal("expected error for empty row id")
	}
}
