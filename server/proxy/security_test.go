package proxy

import (
	"bytes"
	"testing"
)

func TestValidateWebCredentials(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		wantErr  bool
	}{
		{name: "strong", username: "admin", password: "correct-horse-battery-staple", wantErr: false},
		{name: "empty username", username: "", password: "correct-horse-battery-staple", wantErr: true},
		{name: "short password", username: "admin", password: "12345678901", wantErr: true},
		{name: "placeholder", username: "admin", password: "CHANGE_ME_BEFORE_START", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotErr := validateWebCredentials(tt.username, tt.password) != nil; gotErr != tt.wantErr {
				t.Fatalf("validateWebCredentials() error = %v, wantErr %v", gotErr, tt.wantErr)
			}
		})
	}
}

func TestValidateAPISecret(t *testing.T) {
	if err := validateAPISecret(""); err != nil {
		t.Fatalf("empty auth key should keep the API disabled: %v", err)
	}
	if err := validateAPISecret("too-short"); err == nil {
		t.Fatal("short API secret must be rejected")
	}
	if err := validateAPISecret("0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatalf("32-character API secret should be accepted: %v", err)
	}
}

func TestReadSocksAddress(t *testing.T) {
	payload := []byte{domainName, 11}
	payload = append(payload, []byte("example.com")...)
	payload = append(payload, 0x01, 0xbb)
	host, port, err := readSocksAddress(bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if host != "example.com" || port != 443 {
		t.Fatalf("unexpected SOCKS address %s:%d", host, port)
	}
}

func TestReadSocksAddressRejectsTruncatedInput(t *testing.T) {
	if _, _, err := readSocksAddress(bytes.NewReader([]byte{ipV4, 127, 0})); err == nil {
		t.Fatal("expected truncated SOCKS address to fail")
	}
}
