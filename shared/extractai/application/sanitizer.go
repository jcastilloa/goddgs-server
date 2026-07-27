package application

import (
	"bytes"
	"fmt"
	standardHTML "html"
	"strings"

	"github.com/jcastilloa/goddgs-server/shared/extractai/domain"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var allowedTags = map[string]struct{}{
	"article": {}, "main": {}, "section": {}, "div": {}, "p": {}, "br": {}, "hr": {},
	"h1": {}, "h2": {}, "h3": {}, "h4": {}, "h5": {}, "h6": {},
	"ul": {}, "ol": {}, "li": {}, "dl": {}, "dt": {}, "dd": {},
	"blockquote": {}, "pre": {}, "code": {}, "strong": {}, "b": {}, "em": {}, "i": {}, "mark": {}, "small": {}, "sub": {}, "sup": {},
	"a": {}, "img": {}, "figure": {}, "figcaption": {},
	"table": {}, "thead": {}, "tbody": {}, "tfoot": {}, "tr": {}, "th": {}, "td": {}, "caption": {},
}

var removedTags = map[string]struct{}{
	"script": {}, "style": {}, "noscript": {}, "template": {}, "iframe": {}, "object": {}, "embed": {}, "form": {}, "input": {}, "button": {}, "select": {}, "textarea": {},
	"nav": {}, "aside": {}, "header": {}, "footer": {}, "menu": {}, "dialog": {}, "svg": {}, "math": {}, "canvas": {}, "video": {}, "audio": {}, "source": {},
	"base": {}, "link": {}, "meta": {}, "title": {}, "head": {},
}

func SanitizeHTML(content string) (string, error) {
	document, err := html.Parse(strings.NewReader(stripCodeFence(content)))
	if err != nil {
		return "", fmt.Errorf("parse AI HTML response: %w", err)
	}
	body := findBody(document)
	if body == nil {
		return "", domain.ErrInvalidResponse
	}

	var output bytes.Buffer
	for child := body.FirstChild; child != nil; child = child.NextSibling {
		renderNode(&output, child)
	}
	return strings.TrimSpace(output.String()), nil
}

func findBody(node *html.Node) *html.Node {
	if node.Type == html.ElementNode && node.DataAtom == atom.Body {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if body := findBody(child); body != nil {
			return body
		}
	}
	return nil
}

func renderNode(output *bytes.Buffer, node *html.Node) {
	switch node.Type {
	case html.TextNode:
		output.WriteString(standardHTML.EscapeString(node.Data))
	case html.ElementNode:
		renderElement(output, node)
	}
}

func renderElement(output *bytes.Buffer, node *html.Node) {
	tag := strings.ToLower(node.Data)
	if _, removed := removedTags[tag]; removed || hasChromeMarker(node) {
		return
	}
	if _, allowed := allowedTags[tag]; !allowed {
		renderChildren(output, node)
		return
	}
	if tag == "a" && !hasSafeLink(node) {
		renderChildren(output, node)
		return
	}
	if tag == "img" && !hasSafeImage(node) {
		return
	}

	output.WriteByte('<')
	output.WriteString(tag)
	renderAttributes(output, node, tag)
	output.WriteByte('>')
	if tag == "br" || tag == "hr" || tag == "img" {
		return
	}
	renderChildren(output, node)
	output.WriteString("</")
	output.WriteString(tag)
	output.WriteByte('>')
}

func hasChromeMarker(node *html.Node) bool {
	if !isChromeContainer(node.Data) {
		return false
	}
	for _, name := range []string{"id", "class", "role"} {
		if chromeMarker(attribute(node, name)) {
			return true
		}
	}
	return false
}

func isChromeContainer(tag string) bool {
	switch strings.ToLower(tag) {
	case "div", "section", "ul", "ol":
		return true
	default:
		return false
	}
}

func chromeMarker(value string) bool {
	value = strings.ToLower(value)
	for _, marker := range []string{
		"advert", "banner", "cookie", "consent", "sidebar", "navigation", "navbar", "menu", "footer", "header", "related", "recommend", "social", "share", "comment", "subscribe", "paywall", "popup", "modal",
	} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func renderChildren(output *bytes.Buffer, node *html.Node) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		renderNode(output, child)
	}
}

func hasSafeLink(node *html.Node) bool {
	return isSafeURL(attribute(node, "href"), true)
}

func hasSafeImage(node *html.Node) bool {
	return isSafeURL(attribute(node, "src"), false)
}

func renderAttributes(output *bytes.Buffer, node *html.Node, tag string) {
	switch tag {
	case "a":
		writeAttribute(output, "href", attribute(node, "href"))
	case "img":
		writeAttribute(output, "src", attribute(node, "src"))
		writeAttribute(output, "alt", attribute(node, "alt"))
	}
}

func attribute(node *html.Node, name string) string {
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, name) {
			return strings.TrimSpace(attribute.Val)
		}
	}
	return ""
}

func isSafeURL(value string, allowMailto bool) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	lowercase := strings.ToLower(value)
	if strings.HasPrefix(lowercase, "javascript:") || strings.HasPrefix(lowercase, "data:") || strings.HasPrefix(lowercase, "vbscript:") {
		return false
	}
	return allowMailto || !strings.HasPrefix(lowercase, "mailto:")
}

func writeAttribute(output *bytes.Buffer, name, value string) {
	if value == "" {
		return
	}
	output.WriteByte(' ')
	output.WriteString(name)
	output.WriteString(`="`)
	output.WriteString(standardHTML.EscapeString(value))
	output.WriteByte('"')
}

func stripCodeFence(content string) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "```") {
		return content
	}
	firstLineEnd := strings.IndexByte(content, '\n')
	if firstLineEnd == -1 {
		return content
	}
	content = content[firstLineEnd+1:]
	if end := strings.LastIndex(content, "```"); end >= 0 {
		content = content[:end]
	}
	return strings.TrimSpace(content)
}
