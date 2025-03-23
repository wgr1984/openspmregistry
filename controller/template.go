package controller

import "html/template"

type DefaultTemplateParser struct{}

func NewDefaultTemplateParser() *DefaultTemplateParser {
	return &DefaultTemplateParser{}
}

func (p *DefaultTemplateParser) ParseFiles(filenames ...string) (*template.Template, error) {
	return template.ParseFiles(filenames...)
}
