package controllers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	apiTimestampHeader = "X-NPS-Timestamp"
	apiNonceHeader     = "X-NPS-Nonce"
	apiSignatureHeader = "X-NPS-Signature"
	apiClockSkew       = 30 * time.Second
	apiMaxBodySize     = 1 << 20
)

type apiAuthenticator struct {
	mu     sync.Mutex
	nonces map[string]time.Time
}

var requestAPIAuthenticator = &apiAuthenticator{nonces: make(map[string]time.Time)}

func hasAPIAuthHeaders(r *http.Request) bool {
	return r.Header.Get(apiTimestampHeader) != "" ||
		r.Header.Get(apiNonceHeader) != "" ||
		r.Header.Get(apiSignatureHeader) != ""
}

func (a *apiAuthenticator) verify(r *http.Request, secret string, now time.Time) error {
	if strings.TrimSpace(secret) == "" {
		return errors.New("API authentication is disabled")
	}
	timestampText := r.Header.Get(apiTimestampHeader)
	nonce := r.Header.Get(apiNonceHeader)
	signatureText := r.Header.Get(apiSignatureHeader)
	if timestampText == "" || nonce == "" || signatureText == "" {
		return errors.New("missing API authentication headers")
	}
	timestamp, err := strconv.ParseInt(timestampText, 10, 64)
	if err != nil {
		return errors.New("invalid API timestamp")
	}
	requestTime := time.Unix(timestamp, 0)
	if requestTime.Before(now.Add(-apiClockSkew)) || requestTime.After(now.Add(apiClockSkew)) {
		return errors.New("expired API timestamp")
	}
	if len(nonce) < 16 || len(nonce) > 128 {
		return errors.New("invalid API nonce")
	}
	for _, ch := range nonce {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return errors.New("invalid API nonce")
		}
	}
	body, err := readAndRestoreRequestBody(r)
	if err != nil {
		return err
	}
	expected := apiRequestSignature(r, timestampText, nonce, body, secret)
	provided, err := hex.DecodeString(signatureText)
	if err != nil || !hmac.Equal(expected, provided) {
		return errors.New("invalid API signature")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	for key, expiresAt := range a.nonces {
		if !expiresAt.After(now) {
			delete(a.nonces, key)
		}
	}
	if _, exists := a.nonces[nonce]; exists {
		return errors.New("replayed API nonce")
	}
	a.nonces[nonce] = now.Add(apiClockSkew * 2)
	return nil
}

func readAndRestoreRequestBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, apiMaxBodySize+1))
	if err != nil {
		return nil, errors.New("cannot read API request body")
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	if len(body) > apiMaxBodySize {
		return nil, errors.New("API request body is too large")
	}
	return body, nil
}

func apiRequestSignature(r *http.Request, timestamp, nonce string, body []byte, secret string) []byte {
	bodyHash := sha256.Sum256(body)
	requestURI := r.URL.EscapedPath()
	if r.URL.RawQuery != "" {
		requestURI += "?" + r.URL.RawQuery
	}
	canonical := fmt.Sprintf("%s\n%s\n%s\n%s\n%s", r.Method, requestURI, timestamp, nonce, hex.EncodeToString(bodyHash[:]))
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	return mac.Sum(nil)
}
