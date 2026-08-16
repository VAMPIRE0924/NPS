package common

import (
	"strings"
	"testing"
)

func TestAddrRejectsMalformedPackets(t *testing.T) {
	var addr Addr
	for _, payload := range [][]byte{
		nil,
		{ipV4, 127},
		{domainName, 5, 'a'},
		{ipV6, 0},
	} {
		if err := addr.Decode(payload); err == nil {
			t.Fatalf("expected malformed payload %v to fail", payload)
		}
	}
}

func TestAddrRejectsOversizedDomain(t *testing.T) {
	addr := Addr{Type: domainName, Host: strings.Repeat("a", 256), Port: 53}
	if _, err := addr.Encode(make([]byte, 512)); err == nil {
		t.Fatal("expected oversized domain to fail")
	}
}
