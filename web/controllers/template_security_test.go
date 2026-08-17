package controllers

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
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

func TestListTemplatesSurviveStaleLanguageScript(t *testing.T) {
	for _, name := range []string{
		filepath.Join("..", "views", "client", "list.html"),
		filepath.Join("..", "views", "index", "list.html"),
		filepath.Join("..", "views", "index", "hlist.html"),
	} {
		contents, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(contents), "typeof window.npsEscapeHtml === 'function'") {
			t.Fatalf("%s has no HTML-escape fallback for a stale cached language.js", name)
		}
	}
	layout, err := os.ReadFile(filepath.Join("..", "views", "public", "layout.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(layout), "language.js?v=") {
		t.Fatal("language.js URL has no deployment cache buster")
	}
}
