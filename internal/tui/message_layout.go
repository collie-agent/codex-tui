package tui

import (
	"crypto/sha256"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const messageLayoutBudget = 1 << 20
const messageLayoutEntries = 128

type messageLayoutEntry struct {
	index, width int
	version      [32]byte
	lines        [][]string
	bytes        int
}

// Keep one version/width per response, with FIFO eviction and both byte and
// entry limits. Digests avoid retaining replaced source bodies. Eviction affects
// derived rows only; the original response always remains available for rebuild.
func (m Model) responseLayout(index int) [][]string {
	source, width := m.responses[index], m.contentWidth()
	version := sha256.Sum256([]byte(source))
	cache := m.outputCache
	if cache != nil {
		for _, entry := range cache.messages {
			if entry.index == index && entry.width == width && entry.version == version {
				return entry.lines
			}
		}
	}
	lines := make([][]string, 0)
	size := 96
	for _, logical := range strings.Split(renderMarkdown(source, width), "\n") {
		wrapped := strings.Split(ansi.Hardwrap(logical, max(1, width), true), "\n")
		// Own the stored strings rather than pinning a larger renderer buffer.
		for i := range wrapped {
			wrapped[i] = strings.Clone(wrapped[i])
			size += len(wrapped[i]) + 16
		}
		lines = append(lines, wrapped)
		size += 24
	}
	if cache == nil {
		return lines
	}
	// Filter into new storage: a value-model fork may still own the old entries.
	entries := make([]messageLayoutEntry, 0, len(cache.messages)+1)
	used := 0
	for _, entry := range cache.messages {
		if entry.index != index {
			entries = append(entries, entry)
			used += entry.bytes
		}
	}
	if size <= messageLayoutBudget {
		for len(entries) > 0 && (used+size > messageLayoutBudget || len(entries) >= messageLayoutEntries) {
			used -= entries[0].bytes
			entries = entries[1:]
		}
		entries = append(entries, messageLayoutEntry{index: index, width: width, version: version, lines: lines, bytes: size})
	}
	cache.messages = append([]messageLayoutEntry(nil), entries...)
	return lines
}
