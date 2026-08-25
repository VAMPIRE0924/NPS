package bridge

import (
	"testing"

	"ehang.io/nps/lib/file"
)

func TestDisabledConfigUploadStillAcceptsFormalNPCConnection(t *testing.T) {
	client := &file.Client{ConfigConnAllow: false}
	accept, apply := clientConfigAccess(false, client)
	if !accept {
		t.Fatal("formal NPC config startup must be accepted so the legacy NPC can proceed to its main connection")
	}
	if apply {
		t.Fatal("disabled config upload must not add Host or Tunnel rules")
	}
}

func TestEnabledAndPublicConfigUploadRemainApplied(t *testing.T) {
	for _, test := range []struct {
		name   string
		isPub  bool
		client *file.Client
	}{
		{name: "formal client granted by admin", client: &file.Client{ConfigConnAllow: true}},
		{name: "legacy public config mode", isPub: true, client: &file.Client{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			accept, apply := clientConfigAccess(test.isPub, test.client)
			if !accept || !apply {
				t.Fatalf("config access = (%t, %t), want accepted and applied", accept, apply)
			}
		})
	}
}
