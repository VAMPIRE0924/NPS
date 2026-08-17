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
	list, err := os.ReadFile(filepath.Join("..", "views", "index", "list.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(list), "onclick=\\\"submitform") || !strings.Contains(string(list), "data-task-action") {
		t.Fatal("task actions must use data attributes instead of inline JSON event handlers")
	}
}

func TestFileTunnelRemainsAvailableInAddAndEditForms(t *testing.T) {
	for _, name := range []string{
		filepath.Join("..", "views", "index", "add.html"),
		filepath.Join("..", "views", "index", "edit.html"),
	} {
		contents, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(contents), `<option value="file" langtag="scheme-file"></option>`) {
			t.Fatalf("%s does not expose the supported file tunnel mode", name)
		}
	}
}
