package tui

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"charm.land/glamour/v2"
	glamouransi "charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
)

func renderMarkdown(source string, width int) string {
	// Sanitize remote text before rendering; generated ANSI belongs to Glamour.
	source = escapeRemoteControls(source)
	// Plain responses retain their original line boundaries and spacing.
	document := goldmark.DefaultParser().Parse(text.NewReader([]byte(source)))
	plain := true
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch node.Kind() {
			case ast.KindDocument, ast.KindParagraph, ast.KindText:
			default:
				plain = false
			}
		}
		return ast.WalkContinue, nil
	})
	if plain {
		return expandDisplayTabs(source)
	}
	style := githubMarkdownStyle()
	renderer, err := glamour.NewTermRenderer(
		glamour.WithPreservedNewLines(),
		glamour.WithStyles(style),
		glamour.WithWordWrap(max(1, width)))
	if err != nil {
		return expandDisplayTabs(source)
	}
	rendered, err := renderer.Render(source)
	if err != nil {
		return expandDisplayTabs(source)
	}
	return expandDisplayTabs(strings.Trim(rendered, "\n"))
}

// GitHub's restrained dark palette keeps emphasis on content. Terminal headings
// use weight and spacing because terminal cells cannot vary their font size.
func githubMarkdownStyle() glamouransi.StyleConfig {
	style := styles.DarkStyleConfig
	color := func(value string) *string { return &value }
	bold := true
	zero := uint(0)
	style.Document.Margin = &zero
	style.Document.BlockPrefix, style.Document.BlockSuffix = "", ""
	style.Document.Color = color("#e6edf3")
	style.Heading.Color = color("#e6edf3")
	heading := glamouransi.StyleBlock{StylePrimitive: glamouransi.StylePrimitive{Bold: &bold}}
	style.H1, style.H2, style.H3 = heading, heading, heading
	style.H4, style.H5, style.H6 = heading, heading, heading
	style.H6.Color = color("#9198a1")
	style.BlockQuote.Color = color("#9198a1")
	style.Link.Color, style.LinkText.Color = color("#4493f8"), color("#4493f8")
	style.Code.Color = color("#e6edf3")
	style.Code.BackgroundColor = color("#262c36")
	style.CodeBlock.Color = color("#e6edf3")
	style.CodeBlock.BackgroundColor = color("#161b22")
	// Use Chroma's GitHub theme rather than the default neon syntax palette.
	style.CodeBlock.Chroma = nil
	style.CodeBlock.Theme = "github-dark"
	style.HorizontalRule.Color = color("#3d444d")
	return style
}
