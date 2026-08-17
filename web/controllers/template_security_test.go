package controllers

import (
	"html/template"
	"path/filepath"
	"testing"
)

func TestSecuritySensitiveListTemplatesParse(t *testing.T) {
	for _, name := range []string{
		filepath.Join("..", "views", "client", "list.html"),
		filepath.Join("..", "views", "index", "list.html"),
		filepath.Join("..", "views", "index", "hlist.html"),
		filepath.Join("..", "views", "client", "edit.html"),
	} {
		if _, err := template.ParseFiles(name); err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
	}
}
