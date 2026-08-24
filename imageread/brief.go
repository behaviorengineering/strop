package imageread

import "strings"

const (
	SourceVision     = "vision"
	SourcePromptText = "prompt_text"
)

// VisualBrief is a structured description of what appears in the image for downstream prose.
type VisualBrief struct {
	SceneSummary string
	Subjects     string
	Setting      string
	OnImageText  string
	MoodStyle    string
	TeachingBeat string
	Source       string
}

// FormatPlan serializes the brief for post ideation and post generator inputs.
func (b *VisualBrief) FormatPlan() string {
	if b == nil {
		return ""
	}
	var out strings.Builder
	writeLine(&out, "SCENE", b.SceneSummary)
	writeLine(&out, "SUBJECTS", b.Subjects)
	writeLine(&out, "SETTING", b.Setting)
	writeLine(&out, "ON IMAGE TEXT", b.OnImageText)
	writeLine(&out, "MOOD / STYLE", b.MoodStyle)
	writeLine(&out, "TEACHING BEAT", b.TeachingBeat)
	if s := strings.TrimSpace(b.Source); s != "" {
		writeLine(&out, "SOURCE", s)
	}
	return strings.TrimSpace(out.String())
}

func writeLine(b *strings.Builder, label, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	b.WriteString(label)
	b.WriteString(": ")
	b.WriteString(value)
	b.WriteString("\n")
}
