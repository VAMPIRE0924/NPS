package controllers

import (
	"bytes"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	beecontext "github.com/astaxie/beego/context"
)

func signedAPIRequest(t *testing.T, now time.Time, nonce, body, secret string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/index/start?type=socks5", bytes.NewBufferString(body))
	timestamp := strconv.FormatInt(now.Unix(), 10)
	r.Header.Set(apiTimestampHeader, timestamp)
	r.Header.Set(apiNonceHeader, nonce)
	r.Header.Set(apiSignatureHeader, hex.EncodeToString(apiRequestSignature(r, timestamp, nonce, []byte(body), secret)))
	return r
}

func TestAPIAuthenticatorAcceptsSignedBodyAndRejectsReplay(t *testing.T) {
	now := time.Unix(1800000000, 0)
	a := &apiAuthenticator{nonces: make(map[string]time.Time)}
	r := signedAPIRequest(t, now, "nonce-0123456789", "id=1", "test-secret")
	if err := a.verify(r, "test-secret", now); err != nil {
		t.Fatal(err)
	}
	if err := a.verify(r, "test-secret", now); err == nil {
		t.Fatal("expected nonce replay to be rejected")
	}
}

func TestAPIAuthenticatorRejectsTamperingAndExpiredTimestamp(t *testing.T) {
	now := time.Unix(1800000000, 0)
	a := &apiAuthenticator{nonces: make(map[string]time.Time)}
	r := signedAPIRequest(t, now, "nonce-0123456789", "id=1", "test-secret")
	r.Body = ioNopCloserString("id=2")
	if err := a.verify(r, "test-secret", now); err == nil {
		t.Fatal("expected modified body to be rejected")
	}

	r = signedAPIRequest(t, now.Add(-apiClockSkew-time.Second), "nonce-9876543210", "id=1", "test-secret")
	if err := a.verify(r, "test-secret", now); err == nil {
		t.Fatal("expected expired timestamp to be rejected")
	}
}

func TestAPIAuthenticatorUsesBodyCapturedBeforeFormParsing(t *testing.T) {
	now := time.Unix(1800000000, 0)
	a := &apiAuthenticator{nonces: make(map[string]time.Time)}
	r := signedAPIRequest(t, now, "nonce-captured-0123", "id=1&status=true", "test-secret")
	ctx := beecontext.NewContext()
	ctx.Reset(httptest.NewRecorder(), r)
	CaptureAPIRequestBody(ctx)
	if err := ctx.Request.ParseForm(); err != nil {
		t.Fatal(err)
	}
	if err := a.verify(ctx.Request, "test-secret", now); err != nil {
		t.Fatalf("expected captured original form body to verify: %v", err)
	}
}

func TestAPIAuthenticatorRejectsEmptyBodySignatureAfterFormParsing(t *testing.T) {
	now := time.Unix(1800000000, 0)
	a := &apiAuthenticator{nonces: make(map[string]time.Time)}
	r := httptest.NewRequest(http.MethodPost, "/client/list/", bytes.NewBufferString("offset=0&limit=1"))
	timestamp := strconv.FormatInt(now.Unix(), 10)
	nonce := "nonce-empty-body-01"
	r.Header.Set(apiTimestampHeader, timestamp)
	r.Header.Set(apiNonceHeader, nonce)
	r.Header.Set(apiSignatureHeader, hex.EncodeToString(apiRequestSignature(r, timestamp, nonce, nil, "test-secret")))
	ctx := beecontext.NewContext()
	ctx.Reset(httptest.NewRecorder(), r)
	CaptureAPIRequestBody(ctx)
	if err := ctx.Request.ParseForm(); err != nil {
		t.Fatal(err)
	}
	if err := a.verify(ctx.Request, "test-secret", now); err == nil {
		t.Fatal("expected an empty-body signature to be rejected for a non-empty form")
	}
}

func ioNopCloserString(value string) io.ReadCloser {
	return io.NopCloser(bytes.NewBufferString(value))
}
