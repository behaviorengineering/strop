package voice

import (
	"fmt"
	"strings"
	"unicode"
)

// HeuristicAudit checks prose against a caller-supplied profile.
// An empty profile still flags staccato runs. Banned phrases and verb classes apply only when set.
func HeuristicAudit(p Profile, prose string) AuditResult {
	text := strings.TrimSpace(prose)
	result := AuditResult{Pass: true}
	if text == "" {
		return result
	}

	sentences := splitSentences(text)
	counts := make([]int, len(sentences))
	for i, s := range sentences {
		counts[i] = len(strings.Fields(s))
	}
	result.LongestStaccatoRun = longestStaccatoRun(counts)
	if result.LongestStaccatoRun > p.maxStaccatoRun() {
		result.Violations = append(result.Violations, Violation{
			Kind:    "staccato_run",
			Line:    1,
			Excerpt: truncate(strings.Join(sentences, " "), 120),
			Detail:  fmt.Sprintf("staccato run of %d similar short sentences; limit is %d", result.LongestStaccatoRun, p.maxStaccatoRun()),
		})
	}

	for _, phrase := range p.BannedPatterns {
		phrase = strings.TrimSpace(phrase)
		if phrase == "" {
			continue
		}
		if idx := indexFold(text, phrase); idx >= 0 {
			result.Violations = append(result.Violations, Violation{
				Kind:    "banned_phrase",
				Line:    lineAt(text, idx),
				Excerpt: phrase,
				Detail:  "banned phrase from the voice profile",
			})
		}
	}

	if len(p.RequiredVerbs) > 0 {
		for _, para := range splitParagraphs(text) {
			body := strings.TrimSpace(para)
			if body == "" || strings.HasPrefix(body, "#") {
				continue
			}
			if !paragraphHasVerb(body, p.RequiredVerbs) {
				result.ParagraphsMissingVerbs++
				result.Violations = append(result.Violations, Violation{
					Kind:    "missing_verb",
					Line:    lineAt(text, indexFold(text, firstLine(body))),
					Excerpt: truncate(body, 120),
					Detail:  "paragraph has none of the required verb classes",
				})
			}
		}
	}

	result.Pass = len(result.Violations) == 0
	return result
}

// HeuristicFeedback returns checklist lines when the audit fails.
// Empty string means no hit.
func HeuristicFeedback(p Profile, prose string) string {
	result := HeuristicAudit(p, prose)
	if result.Pass {
		return ""
	}
	lines := make([]string, 0, len(result.Violations)+1)
	lines = append(lines, "[ ] voice_fidelity: cadence, banned phrases, or required verbs missed the profile")
	for _, v := range result.Violations {
		lines = append(lines, fmt.Sprintf("[ ] %s line %d: %s", v.Kind, v.Line, v.Detail))
	}
	return strings.Join(lines, "\n")
}

func splitSentences(text string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		s := strings.TrimSpace(b.String())
		b.Reset()
		if s != "" {
			out = append(out, s)
		}
	}
	for _, r := range text {
		b.WriteRune(r)
		if r == '.' || r == '!' || r == '?' {
			flush()
		}
	}
	flush()
	return out
}

func splitParagraphs(text string) []string {
	return strings.Split(text, "\n\n")
}

func longestStaccatoRun(counts []int) int {
	if len(counts) == 0 {
		return 0
	}
	best := 1
	run := 1
	for i := 1; i < len(counts); i++ {
		if similarShort(counts[i-1], counts[i]) {
			run++
			if run > best {
				best = run
			}
			continue
		}
		run = 1
	}
	return best
}

func similarShort(a, b int) bool {
	if a < 6 || a > 14 || b < 6 || b > 14 {
		return false
	}
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= 1
}

func paragraphHasVerb(para string, verbs []string) bool {
	fields := strings.FieldsFunc(para, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-'
	})
	set := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		set[strings.ToLower(f)] = struct{}{}
	}
	for _, verb := range verbs {
		verb = strings.ToLower(strings.TrimSpace(verb))
		if verb == "" {
			continue
		}
		if _, ok := set[verb]; ok {
			return true
		}
	}
	return false
}

func indexFold(text, phrase string) int {
	return strings.Index(strings.ToLower(text), strings.ToLower(phrase))
}

func lineAt(text string, idx int) int {
	if idx < 0 {
		return 1
	}
	return strings.Count(text[:idx], "\n") + 1
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}
