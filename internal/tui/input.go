package tui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

// lineInput keeps the caret on a grapheme boundary, so deleting a character
// never leaves half a Chinese character, combining sequence, or emoji behind.
type lineInput struct {
	text  string
	caret int // byte offset
}

func (in lineInput) previous() int {
	previous := 0
	graphemes := uniseg.NewGraphemes(in.text)
	for graphemes.Next() {
		start, end := graphemes.Positions()
		if end >= in.caret {
			return start
		}
		previous = end
	}
	return previous
}

func (in lineInput) next() int {
	graphemes := uniseg.NewGraphemes(in.text)
	for graphemes.Next() {
		_, end := graphemes.Positions()
		if end > in.caret {
			return end
		}
	}
	return len(in.text)
}

func (in *lineInput) insert(text string) {
	// Pasted text stays on one line; terminal controls must never reach rendering.
	text = strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t', '\u2028', '\u2029':
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
	if text == "" {
		return
	}
	in.text = in.text[:in.caret] + text + in.text[in.caret:]
	in.caret += len(text)
	// Insertion can join adjacent graphemes (for example a combining mark).
	in.alignCaret()
}

func (in *lineInput) alignCaret() {
	if in.caret == 0 {
		return
	}
	graphemes := uniseg.NewGraphemes(in.text)
	for graphemes.Next() {
		_, end := graphemes.Positions()
		if end >= in.caret {
			in.caret = end
			break
		}
	}
}

func (in *lineInput) backspace() {
	start := in.previous()
	in.text = in.text[:start] + in.text[in.caret:]
	in.caret = start
	in.alignCaret()
}

func (in *lineInput) delete() {
	in.text = in.text[:in.caret] + in.text[in.next():]
	in.alignCaret()
}

// display scrolls by complete graphemes and reserves a cell for the caret.
// The stored text survives resizing, even when no screen cells are available.
func (in lineInput) display(width int) (string, int) {
	if width <= 0 {
		return "", 0
	}
	prefix := ansi.Truncate("› ", min(2, width-1), "")
	available := width - ansi.StringWidth(prefix)
	start, caretWidth := 0, ansi.StringWidth(in.text[:in.caret])
	graphemes := uniseg.NewGraphemes(in.text[:in.caret])
	for caretWidth >= available && graphemes.Next() {
		_, start = graphemes.Positions()
		caretWidth -= ansi.StringWidth(graphemes.Str())
	}
	text := in.text[start:]
	if in.text == "" {
		text = "Ask Codex to do anything"
	}
	return prefix + ansi.Truncate(text, available, ""), ansi.StringWidth(prefix) + caretWidth
}
