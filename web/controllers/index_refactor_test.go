package controllers

import (
	"testing"

	"ehang.io/nps/lib/file"
)

func TestCloneTunnelForUpdateDoesNotMutateStoredCompositeKeyFields(t *testing.T) {
	original := &file.Tunnel{
		Id:     1,
		Mode:   file.TaskModePortForward,
		Target: &file.Target{TargetStr: "127.0.0.1:80"},
	}

	updated := cloneTunnelForUpdate(original)
	updated.Mode = "httpProxy"
	updated.Target.TargetStr = "127.0.0.1:8080"

	if original.Mode != file.TaskModePortForward {
		t.Fatalf("stored task mode changed before the old runtime could be stopped: %s", original.Mode)
	}
	if original.Target.TargetStr != "127.0.0.1:80" {
		t.Fatalf("stored task target changed through update clone: %s", original.Target.TargetStr)
	}
}
