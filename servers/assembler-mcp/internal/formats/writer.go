package formats

import (
	"fmt"
	"strings"
)

// Writer converts rendered Markdown content to various output formats
type Writer struct{}

// NewWriter creates a new format writer
func NewWriter() *Writer {
	return &Writer{}
}

// ToDocx converts Markdown content to a simplified DOCX-compatible XML representation.
// For a full production DOCX, use a library like unioffice. This provides a
// readable text representation that can be imported into Word.
func (w *Writer) ToDocx(markdown string) (string, error) {
	// For now, return the markdown with a note about format conversion.
	// A production implementation would use unioffice or similar to create
	// actual .docx files (Open XML format).
	lines := strings.Split(markdown, "\n")
	var docxContent strings.Builder

	docxContent.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	docxContent.WriteString("<document>\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			docxContent.WriteString(fmt.Sprintf("  <heading level=\"1\">%s</heading>\n", strings.TrimPrefix(trimmed, "# ")))
		} else if strings.HasPrefix(trimmed, "## ") {
			docxContent.WriteString(fmt.Sprintf("  <heading level=\"2\">%s</heading>\n", strings.TrimPrefix(trimmed, "## ")))
		} else if strings.HasPrefix(trimmed, "### ") {
			docxContent.WriteString(fmt.Sprintf("  <heading level=\"3\">%s</heading>\n", strings.TrimPrefix(trimmed, "### ")))
		} else if strings.HasPrefix(trimmed, "- ") {
			docxContent.WriteString(fmt.Sprintf("  <listItem>%s</listItem>\n", strings.TrimPrefix(trimmed, "- ")))
		} else if trimmed == "---" {
			docxContent.WriteString("  <horizontalRule/>\n")
		} else if trimmed != "" {
			docxContent.WriteString(fmt.Sprintf("  <paragraph>%s</paragraph>\n", trimmed))
		}
	}

	docxContent.WriteString("</document>\n")
	return docxContent.String(), nil
}

// ToPDF converts Markdown content to a text-based representation.
// For a full production PDF, use a library like gofpdf or maroto.
func (w *Writer) ToPDF(markdown string) (string, error) {
	// Simple text representation for PDF.
	// A production implementation would use gofpdf/maroto to create actual PDF.
	var pdfContent strings.Builder

	pdfContent.WriteString("=== PDF DOCUMENT ===\n\n")

	lines := strings.Split(markdown, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			title := strings.TrimPrefix(trimmed, "# ")
			pdfContent.WriteString(strings.ToUpper(title) + "\n")
			pdfContent.WriteString(strings.Repeat("=", len(title)) + "\n\n")
		} else if strings.HasPrefix(trimmed, "## ") {
			section := strings.TrimPrefix(trimmed, "## ")
			pdfContent.WriteString(section + "\n")
			pdfContent.WriteString(strings.Repeat("-", len(section)) + "\n\n")
		} else if strings.HasPrefix(trimmed, "### ") {
			pdfContent.WriteString(strings.TrimPrefix(trimmed, "### ") + "\n\n")
		} else if trimmed == "---" {
			pdfContent.WriteString("\n" + strings.Repeat("-", 40) + "\n\n")
		} else {
			pdfContent.WriteString(line + "\n")
		}
	}

	pdfContent.WriteString("\n=== END PDF DOCUMENT ===\n")
	return pdfContent.String(), nil
}
