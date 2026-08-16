package server

import (
	"ehang.io/nps/lib/file"
	"testing"
	"time"
)

func TestSocksIdleStateClosesAfterTimeoutWithoutFlowChange(t *testing.T) {
	now := time.Unix(1000, 0)
	state := &socksIdleState{}

	if state.shouldClose(0, 0, now, managedSocksIdleTimeout) {
		t.Fatal("new idle state should not close immediately")
	}
	if state.shouldClose(0, 0, now.Add(managedSocksIdleTimeout-time.Second), managedSocksIdleTimeout) {
		t.Fatal("idle state should not close before timeout")
	}
	if !state.shouldClose(0, 0, now.Add(managedSocksIdleTimeout), managedSocksIdleTimeout) {
		t.Fatal("idle state should close at timeout without flow change")
	}
}

func TestSocksIdleStateFlowChangeResetsTimeout(t *testing.T) {
	now := time.Unix(1000, 0)
	state := &socksIdleState{}

	if state.shouldClose(0, 0, now, managedSocksIdleTimeout) {
		t.Fatal("new idle state should not close immediately")
	}
	if state.shouldClose(10, 0, now.Add(10*time.Minute), managedSocksIdleTimeout) {
		t.Fatal("flow change should reset idle timeout")
	}
	if state.shouldClose(10, 0, now.Add(39*time.Minute), managedSocksIdleTimeout) {
		t.Fatal("idle state should not close before reset timeout")
	}
	if !state.shouldClose(10, 0, now.Add(40*time.Minute), managedSocksIdleTimeout) {
		t.Fatal("idle state should close after reset timeout")
	}
}

func TestNewModeCanonicalizesLegacyPortForwardModes(t *testing.T) {
	for _, mode := range []string{"tcp", "udp"} {
		task := &file.Tunnel{Mode: mode}
		if service := NewMode(nil, task); service == nil {
			t.Fatalf("expected %s to resolve to port-forward service", mode)
		}
		if task.Mode != file.TaskModePortForward {
			t.Fatalf("expected mode %s to canonicalize to %s, got %s", mode, file.TaskModePortForward, task.Mode)
		}
	}
}
