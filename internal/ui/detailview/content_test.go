package detailview

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ContentSuite struct{ suite.Suite }

func TestContentSuite(t *testing.T) { suite.Run(t, new(ContentSuite)) }

func (s *ContentSuite) TestPlainAndCodeStayLiteral() {
	for _, format := range []string{"plain", "code"} {
		s.Run(format, func() {
			got, err := Render(Content{Format: format, Text: `<script>alert("x")</script>&`})
			s.Require().NoError(err)
			s.Equal(`<pre>&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;&amp;</pre>`, got)
		})
	}
}

func (s *ContentSuite) TestMarkdownSyntax() {
	got, err := Render(Content{Format: "markdown", Text: "# Title\n\n**bold** and *italic* with `code`.\n\n" +
		"- one\n- two\n\n1. first\n2. second\n\n> quote\n\n```go\nx < y\n```\n\n[site](https://example.com)"})
	s.Require().NoError(err)
	for _, fragment := range []string{
		"<h1>Title</h1>", "<strong>bold</strong>", "<em>italic</em>", "<code>code</code>", "<ul>", "<ol>",
		"<blockquote>", "<pre><code>x &lt; y\n</code></pre>", `<a href="https://example.com">site</a>`,
	} {
		s.Contains(got, fragment)
	}
}

func (s *ContentSuite) TestImagesAndRawHTMLAreInert() {
	got, err := Render(Content{Format: "markdown", Text: "![alt text](file:///etc/passwd)\n\n" +
		"<img src=\"https://evil.invalid/x\" onerror=\"alert(1)\">"})
	s.Require().NoError(err)
	s.Contains(got, "alt text")
	s.NotContains(got, "<img")
	s.NotContains(got, "file:///etc/passwd")
	s.Contains(got, "&lt;img")
	s.NotContains(got, "onerror=\"alert")
	inline, inlineErr := Render(Content{Format: "markdown", Text: "Text <img src='file:///secret'> end."})
	s.Require().NoError(inlineErr)
	s.Contains(inline, "&lt;img")
	s.NotContains(inline, "<img")
}

func (s *ContentSuite) TestDestinationCannotInjectAttributes() {
	got, err := Render(Content{Format: "markdown", Text: `[x](<https://example.com/"onmouseover="x>)`})
	s.Require().NoError(err)
	s.Contains(got, `href="https://example.com/&#34;onmouseover=&#34;x"`)
	s.NotContains(got, `"onmouseover="`)
}

func (s *ContentSuite) TestLinkDispatchGate() {
	for _, link := range []string{"http://example.com", "https://example.com/path?q=a#fragment"} {
		s.NoError(ValidateLink(link), link)
	}
	for _, link := range []string{
		"", "/relative", "//example.com/path", "javascript:alert(1)", "exec:/bin/true", "unknown:runix",
		"data:text/html,x", "file:///etc/passwd", "https://user:pass@example.com", "https://", "https://:443",
		"https://example.com\n.evil", "https://example.com/%zz",
	} {
		s.Error(ValidateLink(link), link)
	}
}

func (s *ContentSuite) TestUnsupportedFormat() {
	_, err := Render(Content{Format: "html", Text: "<b>unsafe</b>"})
	s.Error(err)
}
