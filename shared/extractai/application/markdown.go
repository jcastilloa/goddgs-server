package application

import (
	"bytes"
	"fmt"

	"github.com/yuin/goldmark"
)

func RenderMarkdownHTML(content string) (string, error) {
	var output bytes.Buffer
	if err := goldmark.Convert([]byte(content), &output); err != nil {
		return "", fmt.Errorf("render extracted Markdown as HTML: %w", err)
	}
	return SanitizeHTML(output.String())
}
