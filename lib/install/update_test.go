package install

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifyReleaseChecksum(t *testing.T) {
	archive := []byte("release archive")
	sum := sha256.Sum256(archive)
	checksums := hex.EncodeToString(sum[:]) + "  nps-linux-amd64-with-web.tar.gz\n"
	if err := verifyReleaseChecksum("nps-linux-amd64-with-web.tar.gz", archive, checksums); err != nil {
		t.Fatal(err)
	}
	if err := verifyReleaseChecksum("nps-linux-amd64-with-web.tar.gz", []byte("tampered"), checksums); err == nil {
		t.Fatal("expected modified archive to fail checksum verification")
	}
}
