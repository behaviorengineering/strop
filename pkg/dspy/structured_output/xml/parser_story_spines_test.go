package xml

import (
	"strings"
	"testing"

	"github.com/XiaoConstantine/dspy-go/pkg/core"
)

func storySignature() core.Signature {
	return core.NewSignature(nil, []core.OutputField{
		{Field: core.NewField("story_titles", core.WithDescription("XML array (list of items): repeat <item> under <story_titles>."), core.WithNoPrefix())},
		{Field: core.NewField("story_start_timestamps", core.WithDescription("XML array (list of items): repeat <item> under <story_start_timestamps>."), core.WithNoPrefix())},
		{Field: core.NewField("story_summaries", core.WithDescription("XML array (list of items): repeat <item> under <story_summaries>."), core.WithNoPrefix())},
		{Field: core.NewField("story_reconstruction_spines", core.WithDescription("XML array (list of items): repeat <item> under <story_reconstruction_spines>. One spine per story, same number and order as story_titles. Each spine item must contain 4-7 beats, one beat per line (newline-separated)."), core.WithNoPrefix())},
	})
}

func TestParseXML_StoryReconstructionSpines_CDATAWithNewlineBeats(t *testing.T) {
	p := NewXMLParser()
	raw := `<response>
<story_titles>
<item><![CDATA[Chicago's Violence and Free Speech]]></item>
<item><![CDATA[Gandalf's Battle Cry: Goodness vs. Power]]></item>
</story_titles>
<story_start_timestamps>
<item><![CDATA[00:02:54]]></item>
<item><![CDATA[00:05:36]]></item>
</story_start_timestamps>
<story_summaries>
<item><![CDATA[A friend explains Chicago's strong free speech culture.]]></item>
<item><![CDATA[Comparing Gandalf's battle cries reveals a shift.]]></item>
</story_summaries>
<story_reconstruction_spines>
    <item>
    <![CDATA[
Friend explains Chicago's free speech
Attributes it to violence and fear in Hyde Park
Violence acts as a reality check for impractical ideas
Machiavelli would agree necessity prevents moral decadence
    ]]>
    </item>
    <item>
    <![CDATA[
Speaker compares Gandalf's battle cries
Original Gandalf uses power-backed words
New Gandalf says "I am good"
Modern writers, knowing only peace, misunderstand good's need for power
This leads to moral decadence and self-righteousness
    ]]>
    </item>
</story_reconstruction_spines>
</response>`

	fields, err := p.parseXML(raw, storySignature(), testXMLConfig())
	if err != nil {
		t.Fatalf("parseXML: %v", err)
	}

	spines, ok := fields["story_reconstruction_spines"].([]interface{})
	if !ok {
		t.Fatalf("expected []interface{} for story_reconstruction_spines, got %T (%#v)", fields["story_reconstruction_spines"], fields["story_reconstruction_spines"])
	}
	if len(spines) != 2 {
		t.Fatalf("expected 2 spine items, got %d: %#v", len(spines), spines)
	}
	first, _ := spines[0].(string)
	if !strings.Contains(first, "Friend explains Chicago's free speech") {
		t.Fatalf("first spine lost beats: %q", first)
	}
	if !strings.Contains(first, "\n") {
		t.Fatalf("first spine should keep newline-separated beats: %q", first)
	}
	second, _ := spines[1].(string)
	if !strings.Contains(second, "Speaker compares Gandalf's battle cries") {
		t.Fatalf("second spine lost beats: %q", second)
	}
	if strings.Contains(first, " >>> ") || strings.Contains(second, " >>> ") {
		t.Fatalf("spines must not use >>> separators: first=%q second=%q", first, second)
	}
}

func TestParseXML_StoryReconstructionSpines_SuffixSpinesWithoutListDescription(t *testing.T) {
	p := NewXMLParser()
	sig := core.NewSignature(nil, []core.OutputField{
		{Field: core.NewField("story_reconstruction_spines", core.WithDescription("one spine per story"), core.WithNoPrefix())},
	})
	raw := `<response><story_reconstruction_spines><item><![CDATA[a
b
c
d]]></item><item><![CDATA[w
x
y
z]]></item></story_reconstruction_spines></response>`
	fields, err := p.parseXML(raw, sig, testXMLConfig())
	if err != nil {
		t.Fatalf("parseXML: %v", err)
	}
	spines, ok := fields["story_reconstruction_spines"].([]interface{})
	if !ok || len(spines) != 2 {
		t.Fatalf("expected 2 spine items via _spines suffix, got %T %#v", fields["story_reconstruction_spines"], fields["story_reconstruction_spines"])
	}
}

func TestParseXML_StoryReconstructionSpines_PlainStringFieldStillCollectsItemText(t *testing.T) {
	p := NewXMLParser()
	sig := core.NewSignature(nil, []core.OutputField{
		{Field: core.NewField("story_titles", core.WithDescription("XML array (list of items)"), core.WithNoPrefix())},
		{Field: core.NewField("story_start_timestamps", core.WithDescription("XML array (list of items)"), core.WithNoPrefix())},
		{Field: core.NewField("story_summaries", core.WithDescription("XML array (list of items)"), core.WithNoPrefix())},
		{Field: core.NewField("story_reconstruction_spines", core.WithDescription("reconstruction spine"), core.WithNoPrefix())},
	})
	raw := `<response>
<story_titles><item><![CDATA[A]]></item></story_titles>
<story_start_timestamps><item><![CDATA[00:00:00]]></item></story_start_timestamps>
<story_summaries><item><![CDATA[sum]]></item></story_summaries>
<story_reconstruction_spines>
    <item>
    <![CDATA[
Friend explains Chicago's free speech
Attributes it to violence and fear in Hyde Park
Violence acts as a reality check for impractical ideas
Machiavelli would agree necessity prevents moral decadence
    ]]>
    </item>
</story_reconstruction_spines>
</response>`
	fields, err := p.parseXML(raw, sig, testXMLConfig())
	if err != nil {
		t.Fatalf("parseXML: %v", err)
	}
	t.Logf("spines type=%T value=%#v", fields["story_reconstruction_spines"], fields["story_reconstruction_spines"])
}
