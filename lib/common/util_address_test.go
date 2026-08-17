package common

import (
	"encoding/base64"
	"net/http/httptest"
	"testing"
)

func TestAddressHelpersSupportIPv6(t *testing.T) {
	if got := GetIpByAddr("[2001:db8::1]:8024"); got != "2001:db8::1" {
		t.Fatalf("unexpected IPv6 host %q", got)
	}
	if got := GetPortByAddr("[2001:db8::1]:8024"); got != 8024 {
		t.Fatalf("unexpected IPv6 port %d", got)
	}
	if got := GetIpByAddr("example.com:80"); got != "example.com" {
		t.Fatalf("unexpected hostname %q", got)
	}
}

func TestProxyAuthorizationIsPreferredAndStrippedWithoutLosingOriginAuth(t *testing.T) {
	r := httptest.NewRequest("GET", "http://example.com", nil)
	r.Header.Set("Authorization", "Bearer origin-token")
	r.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("proxy-user:proxy-password")))
	user, password, ok := GetAuth(r)
	if !ok || user != "proxy-user" || password != "proxy-password" {
		t.Fatalf("unexpected proxy credentials %q %q %v", user, password, ok)
	}
	StripProxyCredentials(r)
	if r.Header.Get("Proxy-Authorization") != "" || r.Header.Get("Authorization") != "Bearer origin-token" {
		t.Fatal("proxy credential stripping removed origin authorization")
	}
}
