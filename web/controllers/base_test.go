package controllers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUnauthenticatedProtectedPostRequiresAPIAuth(t *testing.T) {
	post := httptest.NewRequest(http.MethodPost, "/client/list/", nil)
	if !unauthenticatedRequestRequiresAPIAuth(post) {
		t.Fatal("expected an unauthenticated protected POST to require API authentication")
	}
	get := httptest.NewRequest(http.MethodGet, "/client/list/", nil)
	if unauthenticatedRequestRequiresAPIAuth(get) {
		t.Fatal("expected an unauthenticated page GET to retain the login redirect flow")
	}
}
