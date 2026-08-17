package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebSecurityHandlerHardensHeadersAndCookies(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "secret", HttpOnly: true})
		w.WriteHeader(http.StatusNoContent)
	})
	recorder := httptest.NewRecorder()
	newWebSecurityHandler(next, true).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	result := recorder.Result()
	if result.Header.Get("X-Content-Type-Options") != "nosniff" || result.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatal("missing browser security headers")
	}
	if result.Header.Get("Strict-Transport-Security") == "" {
		t.Fatal("TLS web server must emit HSTS")
	}
	cookie := result.Header.Get("Set-Cookie")
	if !strings.Contains(cookie, "SameSite=Lax") || !strings.Contains(cookie, "Secure") || !strings.Contains(cookie, "HttpOnly") {
		t.Fatalf("cookie was not hardened: %s", cookie)
	}
}

func TestWebSecurityHandlerDoesNotMarkPlainHTTPCookieSecure(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "secret"})
	})
	recorder := httptest.NewRecorder()
	newWebSecurityHandler(next, false).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	cookie := recorder.Result().Header.Get("Set-Cookie")
	if strings.Contains(cookie, "; Secure") || !strings.Contains(cookie, "SameSite=Lax") {
		t.Fatalf("unexpected plain HTTP cookie attributes: %s", cookie)
	}
}
