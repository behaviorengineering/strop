package xml

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

func TestRepairUnclosedElementTags_ClosesTruncatedResponse(t *testing.T) {
	in := `<response>
<story_titles><item>Alpha</item><item>Beta</item></story_titles>
<story_start_timestamps><item>00:00:00</item><item>00:10:00</item></story_start_timestamps>
<story_summaries><item>First summary.</item><item>Second summary cut off mid`
	got := repairUnclosedElementTags(in)
	if !strings.Contains(got, "</story_summaries>") {
		t.Fatalf("expected closed story_summaries, got:\n%s", got)
	}
	if !strings.Contains(got, "</response>") {
		t.Fatalf("expected closed response, got:\n%s", got)
	}
}

func TestRepairUnclosedElementTags_LeavesCompleteDocumentUnchanged(t *testing.T) {
	in := `<response><story_titles><item>A</item></story_titles></response>`
	if got := repairUnclosedElementTags(in); got != in {
		t.Fatalf("well-formed document changed:\nwant %q\ngot  %q", in, got)
	}
}

func TestRepairUnclosedElementTags_StripsIncompleteTrailingTag(t *testing.T) {
	in := `<response><story_titles><item>Title</item></story_titles><story_summaries><item>Sum`
	got := repairUnclosedElementTags(in)
	if strings.Contains(got, "<item") && !strings.Contains(got, "<item>") {
		t.Fatalf("incomplete trailing tag should be stripped, got:\n%s", got)
	}
	if !strings.HasSuffix(strings.TrimSpace(got), "</response>") {
		t.Fatalf("expected closed response, got:\n%s", got)
	}
}

func TestParseOutputs_TruncatedXML_StrictParsingFails(t *testing.T) {
	parser := NewXMLParser()
	sig := core.NewSignature(nil, []core.OutputField{
		{Field: core.Field{Name: "chapter_titles", Description: "XML array (list of items): titles"}},
		{Field: core.Field{Name: "chapter_start_timestamps", Description: "XML array (list of items): starts"}},
		{Field: core.Field{Name: "chapter_summaries", Description: "XML array (list of items): summaries"}},
	})
	truncated := `<response>
<chapter_titles><item>One</item><item>Two</item></chapter_titles>
<chapter_start_timestamps><item>00:00:00</item><item>00:05:00</item></chapter_start_timestamps>
<chapter_summaries><item>Summary one.</item><item>Summary two without close`
	_, err := parser.ParseOutputs(context.Background(), map[string]any{
		"response": truncated,
	}, sig, XMLConfig{
		StrictParsing: true,
		MaxSize:       1024 * 1024,
		MaxDepth:      15,
		ParseTimeout:  5 * time.Second,
	})
	if err == nil {
		t.Fatal("expected truncated XML to fail under StrictParsing")
	}
	if !strings.Contains(err.Error(), "XML truncated: missing </response>") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseOutputs_TruncatedStoryXML(t *testing.T) {
	parser := NewXMLParser()
	sig := core.NewSignature(nil, []core.OutputField{
		{Field: core.Field{Name: "story_titles", Description: "XML array (list of items): titles"}},
		{Field: core.Field{Name: "story_start_timestamps", Description: "XML array (list of items): starts"}},
		{Field: core.Field{Name: "story_summaries", Description: "XML array (list of items): summaries"}},
		{Field: core.Field{Name: "story_reconstruction_spines", Description: "XML array (list of items): spines"}},
	})
	truncated := `<response>
<story_titles><item>One</item><item>Two</item></story_titles>
<story_start_timestamps><item>00:00:00</item><item>00:05:00</item></story_start_timestamps>
<story_summaries><item>Summary one.</item><item>Summary two without close`
	outputs, err := parser.ParseOutputs(context.Background(), map[string]any{
		"response": truncated,
	}, sig, XMLConfig{
		MaxSize:      1024 * 1024,
		MaxDepth:     15,
		ParseTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("ParseOutputs truncated story XML: %v", err)
	}
	titlesRaw, ok := outputs["story_titles"].([]interface{})
	if !ok {
		t.Fatalf("story_titles type %T", outputs["story_titles"])
	}
	if len(titlesRaw) != 2 {
		t.Fatalf("titles count = %d, want 2", len(titlesRaw))
	}
	summariesRaw, ok := outputs["story_summaries"].([]interface{})
	if !ok {
		t.Fatalf("story_summaries type %T", outputs["story_summaries"])
	}
	if len(summariesRaw) != 2 {
		t.Fatalf("summaries count = %d, want 2", len(summariesRaw))
	}
	if s, _ := summariesRaw[1].(string); !strings.Contains(s, "without close") {
		t.Fatalf("second summary = %q", summariesRaw[1])
	}
}
