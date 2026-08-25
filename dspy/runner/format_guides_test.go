package runner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatRetrievedGuides(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", FormatRetrievedGuides(nil))
	assert.Equal(t, "", FormatRetrievedGuides([]string{"", "  "}))
	assert.Equal(t, "<item>Keep the pun gloss</item>\n<item>Lead with contrast</item>", FormatRetrievedGuides([]string{
		"Keep the pun gloss",
		"Lead with contrast",
	}))
}
