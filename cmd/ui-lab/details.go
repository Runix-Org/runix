package main

import "strings"

// Detail data is ordinary content; parsing and resource policy belong to the UI adapter.
func detailFixture(id string) (string, string) {
	if strings.HasSuffix(id, "00001") {
		return "plain", "Plain text · Привет · 日本語 · 🚀\n\n" +
			"<b>Not bold</b> & <img src=\"https://example.invalid/image.png\">\n**Not Markdown**\n\n" +
			"Select this controlled fixture and copy it."
	}
	if strings.HasSuffix(id, "00002") {
		return "code", "package main\n\n// <b>literal</b> & **literal** · λ\n" +
			"func main() {\n\tprintln(\"Hello, 世界\")\n}\n"
	}
	if strings.HasSuffix(id, "00003") {
		return "markdown", "# Long document\n\n" + strings.Repeat(
			"## Scroll section\n\nUnicode: Привет · 日本語 · 🚀. Selectable **bold** and `code <tag>`.\n\n"+
				"- First item\n- Second item\n\n", 120) + "End of controlled long document.\n"
	}
	return "markdown", "# Native details\n\n" + id + "\n\n" +
		"## Text and code\n\n**Bold**, *emphasis*, `inline <code> & text`. Unicode: Привет · 日本語 · 🚀.\n\n" +
		"- First item\n- Second item\n\n1. Ordered one\n2. Ordered two\n\n" +
		"```go\n// <b>literal</b> & **not bold**\nprintln(\"Hello, 世界\")\n```\n\n" +
		"[Allowed HTTPS](https://example.invalid/runix) · [Allowed HTTP](http://example.invalid/runix)\n\n" +
		"[Reject file](file:///tmp/runix-detail) · [Reject script](javascript:alert%281%29) · " +
		"[Reject executable](exec:/bin/true) · [Reject unknown](unknown:runix)\n\n" +
		"![Remote image must not load](https://example.invalid/remote.png)\n\n" +
		"<img src=\"file:///tmp/runix-detail-image.png\">\n\n" +
		"Select records 00001, 00002, and 00003 for plain text, code, and long Markdown.\n"
}
