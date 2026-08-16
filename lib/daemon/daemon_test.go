package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadPIDValidatesFileContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nps.pid")
	if err := os.WriteFile(path, []byte("1234\n"), 0600); err != nil {
		t.Fatal(err)
	}
	pid, err := readPID("nps", dir)
	if err != nil || pid != 1234 {
		t.Fatalf("unexpected pid result: pid=%d err=%v", pid, err)
	}
	if err := os.WriteFile(path, []byte("1234; touch /tmp/injected"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readPID("nps", dir); err == nil {
		t.Fatal("pid file command injection payload must be rejected")
	}
}
