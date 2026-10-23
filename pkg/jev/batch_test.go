package jev

import (
	"testing"
)

func TestBatchRows_sameOptionSetOneRequest(t *testing.T) {
	q := Question{Type: "noul", Instructions: "score"}
	rows := []Row{
		{ID: "a", StateSuffix: "text a", Questions: map[string]Question{"skip": q, "use:x": q, "use:y": q}},
		{ID: "b", StateSuffix: "text b", Questions: map[string]Question{"skip": q, "use:x": q, "use:y": q}},
	}
	reqs, err := BatchRows(rows, "world", 64)
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 1 {
		t.Fatalf("requests: %d", len(reqs))
	}
	if len(reqs[0].Questions) != 6 {
		t.Fatalf("questions: %d", len(reqs[0].Questions))
	}
}

func TestBatchRows_differentSetsTwoRequests(t *testing.T) {
	q := Question{Type: "noul"}
	rows := []Row{
		{ID: "a", StateSuffix: "t", Questions: map[string]Question{"skip": q}},
		{ID: "b", StateSuffix: "t", Questions: map[string]Question{"skip": q, "use:x": q}},
	}
	reqs, err := BatchRows(rows, "", 64)
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 2 {
		t.Fatalf("requests: %d", len(reqs))
	}
}

func TestBatchRows_chunksAt64(t *testing.T) {
	q := Question{Type: "noul"}
	var rows []Row
	for i := 0; i < 33; i++ {
		rows = append(rows, Row{
			ID:          "row-" + string(rune('A'+i)),
			StateSuffix: "t",
			Questions:   map[string]Question{"o1": q, "o2": q},
		})
	}
	reqs, err := BatchRows(rows, "w", 64)
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(reqs))
	}
	total := 0
	for _, r := range reqs {
		total += len(r.Questions)
	}
	if total != 66 {
		t.Fatalf("total questions: %d", total)
	}
}
