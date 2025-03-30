package controller

import "html/template"

// TemplateParser defines the interface for parsing template files.
// It abstracts the template parsing functionality to allow for mocking in tests
// and flexibility in implementation.
type TemplateParser interface {
	// ParseFiles parses the named files and associates the resulting templates.
	// If an error occurs, parsing stops and the returned template is nil.
	//
	// Parameters:
	//   - filenames: One or more template file paths to parse
	//
	// Returns:
	//   - *template.Template: The parsed template(s)
	//   - error: Non-nil if parsing fails
	ParseFiles(filenames ...string) (*template.Template, error)
}

// DefaultTemplateParser provides the default implementation of TemplateParser
// using the standard html/template package.
type DefaultTemplateParser struct{}

// NewDefaultTemplateParser creates a new instance of DefaultTemplateParser.
func NewDefaultTemplateParser() *DefaultTemplateParser {
	return &DefaultTemplateParser{}
}

// ParseFiles implements the TemplateParser interface using html/template.ParseFiles.
func (p *DefaultTemplateParser) ParseFiles(filenames ...string) (*template.Template, error) {
	return template.ParseFiles(filenames...)
}
