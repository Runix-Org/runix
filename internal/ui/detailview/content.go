// Package detailview renders untrusted detail content as safe Qt RichText HTML.
package detailview

import (
	"bytes"
	"fmt"
	"html"
	"net/url"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Content contains the source text and its explicit rendering format.
type Content struct {
	Format string
	Text   string
}

// ValidateLink is the dispatch gate for links: only absolute HTTP(S) URLs with
// a host are accepted. User information and control characters are rejected.
func ValidateLink(raw string) error {
	for _, r := range raw {
		if unicode.IsControl(r) {
			return fmt.Errorf("link contains a control character")
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid link: %w", err)
	}
	if !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return fmt.Errorf("link must be an absolute HTTP(S) URL with a host and no user information")
	}
	return nil
}

// Render returns escaped, allowlisted Qt RichText HTML. Markdown supports
// headings, paragraphs, ordered/unordered lists, strong/emphasis, inline and
// fenced/indented code, blockquotes, and links. Images are alt text only. Raw
// HTML is emitted literally as escaped text. Unknown AST nodes are reduced to
// their textual descendants; no source markup or resource attributes survive.
func Render(content Content) (string, error) {
	switch content.Format {
	case "plain":
		return "<pre>" + html.EscapeString(content.Text) + "</pre>", nil
	case "code":
		return "<pre>" + html.EscapeString(content.Text) + "</pre>", nil
	case "markdown":
		return renderMarkdown(content.Text), nil
	default:
		return "", fmt.Errorf("unsupported detail format %q", content.Format)
	}
}

func renderMarkdown(source string) string {
	data := []byte(source)
	doc := goldmark.New().Parser().Parse(text.NewReader(data))
	var out bytes.Buffer
	var emit func(ast.Node)
	children := func(n ast.Node) {
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			emit(child)
		}
	}
	emit = func(n ast.Node) {
		switch node := n.(type) {
		case *ast.Document:
			children(node)
		case *ast.Text:
			out.WriteString(html.EscapeString(string(node.Segment.Value(data))))
			if node.HardLineBreak() {
				out.WriteString("<br>")
			} else if node.SoftLineBreak() {
				out.WriteByte('\n')
			}
		case *ast.String:
			out.WriteString(html.EscapeString(string(node.Value)))
		case *ast.Heading:
			tag := fmt.Sprintf("h%d", node.Level)
			out.WriteString("<" + tag + ">")
			children(node)
			out.WriteString("</" + tag + ">")
		case *ast.Paragraph:
			out.WriteString("<p>")
			children(node)
			out.WriteString("</p>")
		case *ast.Emphasis:
			tag := "em"
			if node.Level == 2 {
				tag = "strong"
			}
			out.WriteString("<" + tag + ">")
			children(node)
			out.WriteString("</" + tag + ">")
		case *ast.CodeSpan:
			out.WriteString("<code>")
			children(node)
			out.WriteString("</code>")
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			out.WriteString("<pre><code>")
			for i := range n.Lines().Len() {
				segment := n.Lines().At(i)
				out.WriteString(html.EscapeString(string(segment.Value(data))))
			}
			out.WriteString("</code></pre>")
		case *ast.Link:
			out.WriteString(`<a href="` + html.EscapeString(string(node.Destination)) + `">`)
			children(node)
			out.WriteString("</a>")
		case *ast.Image:
			children(node)
		case *ast.List:
			tag := "ul"
			if node.IsOrdered() {
				tag = "ol"
			}
			out.WriteString("<" + tag + ">")
			children(node)
			out.WriteString("</" + tag + ">")
		case *ast.ListItem:
			out.WriteString("<li>")
			children(node)
			out.WriteString("</li>")
		case *ast.Blockquote:
			out.WriteString("<blockquote>")
			children(node)
			out.WriteString("</blockquote>")
		case *ast.RawHTML:
			for i := range node.Segments.Len() {
				segment := node.Segments.At(i)
				out.WriteString(html.EscapeString(string(segment.Value(data))))
			}
		case *ast.HTMLBlock:
			for i := range node.Lines().Len() {
				segment := node.Lines().At(i)
				out.WriteString(html.EscapeString(string(segment.Value(data))))
			}
			if node.HasClosure() {
				out.WriteString(html.EscapeString(string(node.ClosureLine.Value(data))))
			}
		default:
			children(n)
		}
	}
	emit(doc)
	return out.String()
}
