package helper

import (
	"bytes"
	"embed"
	"html/template"
)

func GenerateEmailBody(templateFS embed.FS, templateName string, data any) (string, error) {
	t, err := template.ParseFS(templateFS, templateName)
	if err != nil {
		return "", err
	}

	var body bytes.Buffer
	if err := t.Execute(&body, data); err != nil {
		return "", err
	}

	return body.String(), nil
}
