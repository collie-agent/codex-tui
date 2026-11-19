package tui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// lineInput keeps the caret on a grapheme boundary, so deleting a character
// never leaves half a Chinese character, combining sequence, or emoji behind.
type lineInput struct {
	text               string
	caret              int // byte offset
	goalColumn         int
	vertical, atRowEnd bool
	pendingKillEnd     int // suffix end plus one, or zero when there is no pending suffix
}

func (in lineInput) previous() int {
	for start := 0; start < in.caret; {
		cluster, _ := ansi.FirstGraphemeCluster(in.text[start:], ansi.GraphemeWidth)
		end := start + len(cluster)
		if end >= in.caret {
			return start
		}
		start = end
	}
	return 0
}

func (in lineInput) next() int {
	cluster, _ := ansi.FirstGraphemeCluster(in.text[in.caret:], ansi.GraphemeWidth)
	return in.caret + len(cluster)
}

func (in *lineInput) insert(text string) {
	in.pendingKillEnd = 0
	// Pasted line separators stay spaces, and terminal controls never reach rendering.
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
	in.insertDraft(text)
}

func (in *lineInput) insertDraft(text string) {
	if text == "" {
		return
	}
	in.vertical, in.atRowEnd = false, false
	in.text = in.text[:in.caret] + text + in.text[in.caret:]
	in.caret += len(text)
	// Insertion can join adjacent graphemes (for example a combining mark).
	in.alignCaret()
}

func (in *lineInput) alignCaret() {
	end := 0
	for end < in.caret {
		cluster, _ := ansi.FirstGraphemeCluster(in.text[end:], ansi.GraphemeWidth)
		end += len(cluster)
	}
	in.caret = end
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

type inputRow struct{ start, end int }

type wrappedInput struct {
	rows             []inputRow
	prefix           string
	caretRow, column int
}

func (in lineInput) wrap(width int) wrappedInput {
	// Drop the prompt before it crowds out a double-width grapheme.
	prefix := ansi.Truncate("› ", max(0, width-2), "")
	// Reserve an insertion cell so a caret at a soft line end is visible.
	available := width - ansi.StringWidth(prefix)
	limit := max(1, available-1)
	w := wrappedInput{prefix: prefix}
	start, col := 0, 0
	// Use the renderer's grapheme boundaries and cell widths throughout editing.
	for pos := 0; pos < len(in.text); {
		cluster, cells := ansi.FirstGraphemeCluster(in.text[pos:], ansi.GraphemeWidth)
		end := pos + len(cluster)
		// A glyph may consume every available cell on a tiny screen.
		// Its trailing insertion point belongs at column zero of the next row.
		if col > 0 && col >= available {
			w.rows = append(w.rows, inputRow{start, pos})
			start, col = pos, 0
		}
		if cluster == "\n" {
			w.rows = append(w.rows, inputRow{start, pos})
			start, col = end, 0
			pos = end
			continue
		}
		if col > 0 && col+cells > limit {
			w.rows = append(w.rows, inputRow{start, pos})
			start, col = pos, 0
		}
		col += cells
		pos = end
	}
	if col > 0 && col >= available {
		w.rows = append(w.rows, inputRow{start, len(in.text)})
		start = len(in.text)
	}
	w.rows = append(w.rows, inputRow{start, len(in.text)})
	for i, r := range w.rows {
		// Both sides of a soft break share a byte offset. Keep the side
		// chosen by line-end or vertical navigation until the next edit.
		if in.caret < r.end || i == len(w.rows)-1 || (in.caret == r.end && (in.atRowEnd || w.rows[i+1].start > r.end)) {
			column := ansi.StringWidth(in.text[r.start:in.caret])
			if column > 0 && column >= available && i+1 < len(w.rows) {
				continue
			}
			w.caretRow = i
			w.column = column
			break
		}
	}
	return w
}

// Keep one layout for the current draft and width. Rendering, viewport geometry
// and the terminal cursor all read the same immutable rows.
type inputLayoutCache struct {
	input   lineInput
	width   int
	wrapped wrappedInput
}

func (m Model) wrappedInput() wrappedInput {
	width := m.contentWidth()
	cache := m.inputCache
	if cache == nil {
		return m.input.wrap(width)
	}
	if cache.wrapped.rows == nil || cache.width != width || cache.input != m.input {
		*cache = inputLayoutCache{input: m.input, width: width, wrapped: m.input.wrap(width)}
	}
	return cache.wrapped
}

func (in *lineInput) resetVertical() { in.vertical = false }

func (in *lineInput) edit(key, text string) {
	in.pendingKillEnd = 0
	in.resetVertical()
	switch key {
	case "left":
		in.caret = in.previous()
	case "right":
		in.caret = in.next()
	case "home":
		in.caret = 0
	case "end":
		in.caret = len(in.text)
	case "backspace":
		in.backspace()
	case "delete":
		in.delete()
	case "ctrl+j":
		// Only an explicit line break bypasses paste normalization.
		in.insertDraft("\n")
		return
	default:
		in.insert(text)
		return
	}
	in.atRowEnd = false
}

func (in *lineInput) navigate(key string, w wrappedInput) {
	row := w.rows[w.caretRow]
	pendingKillEnd := in.pendingKillEnd - 1
	in.pendingKillEnd = 0
	switch key {
	case "ctrl+a":
		in.resetVertical()
		in.caret = row.start
		in.atRowEnd = false
	case "ctrl+e":
		in.resetVertical()
		in.caret = row.end
		in.atRowEnd = true
	case "ctrl+u":
		if row.start == in.caret {
			return
		}
		// Keep the original suffix boundary for an immediate Ctrl+K.
		// Reflow can pull unrelated text into the current row.
		in.pendingKillEnd = row.end - (in.caret - row.start) + 1
		in.text = in.text[:row.start] + in.text[in.caret:]
		in.caret = row.start
		in.alignCaret()
		in.resetVertical()
		in.atRowEnd = false
	case "ctrl+k":
		end := row.end
		if pendingKillEnd >= 0 {
			// The first deletion can join graphemes across the splice.
			// Stop at the last whole grapheme within the original suffix.
			end = in.caret
			for end < pendingKillEnd {
				cluster, _ := ansi.FirstGraphemeCluster(in.text[end:], ansi.GraphemeWidth)
				if end+len(cluster) > pendingKillEnd {
					break
				}
				end += len(cluster)
			}
		}
		if in.caret == end {
			return
		}
		in.text = in.text[:in.caret] + in.text[end:]
		// An emptied row still owns its leading insertion point when
		// the next row reflows into that position.
		in.atRowEnd = in.caret != row.start
		in.alignCaret()
		in.resetVertical()
	default:
		if !in.vertical {
			in.goalColumn = w.column
		}
		in.vertical = true
		target := w.caretRow + 1
		if key == "up" {
			target = w.caretRow - 1
		}
		if target < 0 || target >= len(w.rows) {
			return
		}
		row = w.rows[target]
		in.caret = row.start
		col := 0
		for in.caret < row.end {
			cluster, cells := ansi.FirstGraphemeCluster(in.text[in.caret:row.end], ansi.GraphemeWidth)
			col += cells
			if col > in.goalColumn {
				break
			}
			in.caret += len(cluster)
		}
		in.atRowEnd = in.caret == row.end
	}
}

func (w wrappedInput) window(height int) (first, end, padding int) {
	// On short terminals, trade padding for a second editing row where possible.
	capacity := min(len(w.rows), max(min(2, height), height-2))
	first = max(0, w.caretRow-capacity+1)
	return first, first + capacity, (height - capacity) / 2
}
