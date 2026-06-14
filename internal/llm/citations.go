package llm

import (
	"fmt"
	"strings"
)

// dedupeCitations returns the citations in first-seen order with duplicate URLs
// removed. Entries with an empty URL are dropped.
func dedupeCitations(cites []Citation) []Citation {
	if len(cites) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(cites))
	out := make([]Citation, 0, len(cites))
	for _, c := range cites {
		if c.URL == "" {
			continue
		}
		if _, ok := seen[c.URL]; ok {
			continue
		}
		seen[c.URL] = struct{}{}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// appendSources appends a Markdown "Sources" section listing cites to text. When
// cites is empty the text is returned unchanged. A citation with an empty Title
// falls back to its URL as the link text.
func appendSources(text string, cites []Citation) string {
	if len(cites) == 0 {
		return text
	}
	var b strings.Builder
	b.WriteString(text)
	if text != "" {
		b.WriteString("\n\n")
	}
	b.WriteString("**Sources:**\n")
	for i, c := range cites {
		title := c.Title
		if title == "" {
			title = c.URL
		}
		fmt.Fprintf(&b, "%d. [%s](%s)\n", i+1, title, c.URL)
	}
	return b.String()
}
