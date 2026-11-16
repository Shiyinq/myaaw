package provider

import (
	"regexp"
	"strings"
)

// A selections block carries excerpts the user highlighted in earlier replies
// (an answer, a thinking block, or tool output) and attached to their next
// message:
//
//	<selections note="…">
//	<selection n="1" from="answer">…</selection>
//	<selection n="2" from="tool:bash">…</selection>
//	</selections>
//
// The block is model-only. Clients show a count chip and hide the text; titles,
// previews, and turn labels strip it with StripSelectionsBlocks.
//
// Only the outer plural tag is meaningful. A bare <selection> is ordinary data,
// and no other context tag nests inside this one.
const (
	SelectionsBlockOpen  = "<selections"
	SelectionsBlockClose = "</selections>"
)

var (
	selectionsBlockPattern = regexp.MustCompile(`(?s)<selections\b[^>]*>.*?</selections>[ \t]*\n?`)
	// The word boundary keeps `<selections>` itself from matching the item tag.
	selectionItemPattern = regexp.MustCompile(`<selection\b`)
)

// StripSelectionsBlocks removes every quoted-excerpt block from text. A block
// without a nested <selection> is not one of ours, so it is left alone.
func StripSelectionsBlocks(text string) string {
	if !strings.Contains(text, SelectionsBlockOpen) || !selectionItemPattern.MatchString(text) {
		return text
	}
	return strings.TrimSpace(selectionsBlockPattern.ReplaceAllString(text, ""))
}
