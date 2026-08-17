package common

import (
	"bytes"
	"encoding/binary"
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

func TestReadUDPDatagramRejectsMalformedLengths(t *testing.T) {
	oversized := []byte{0xff, 0xff, 0, ipV4, 127}
	if _, err := ReadUDPDatagram(bytes.NewReader(oversized)); err == nil {
		t.Fatal("expected oversized extended UDP datagram to fail")
	}
	truncated := []byte{0, 0, 0, ipV6, 0}
	if _, err := ReadUDPDatagram(bytes.NewReader(truncated)); err == nil {
		t.Fatal("expected truncated standard UDP datagram to fail")
	}

	tooLarge := make([]byte, PoolSizeUdp+1)
	binary.BigEndian.PutUint16(tooLarge[:2], 0)
	tooLarge[3] = ipV4
	if _, err := ReadUDPDatagram(bytes.NewReader(tooLarge)); err == nil {
		t.Fatal("expected oversized standard UDP datagram to fail")
	}
}

func TestAddrRejectsOversizedDomain(t *testing.T) {
	addr := Addr{Type: domainName, Host: strings.Repeat("a", 256), Port: 53}
	if _, err := addr.Encode(make([]byte, 512)); err == nil {
		t.Fatal("expected oversized domain to fail")
	}
}
