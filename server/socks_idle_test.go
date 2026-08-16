package server

import (
	"testing"
	"time"

	"ehang.io/nps/lib/file"
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

func TestSocksIdleStateActivityDoesNotResetTimeout(t *testing.T) {
	now := time.Unix(1000, 0)
	state := &socksIdleState{}
	state.shouldClose(10, 20, now, managedSocksIdleTimeout)

	idle := state.activity(10, 20, now.Add(10*time.Minute), managedSocksIdleTimeout)
	if idle.active {
		t.Fatal("unchanged flow should be reported as inactive")
	}
	if idle.idle != 10*time.Minute || idle.remaining != 20*time.Minute {
		t.Fatalf("unexpected idle countdown: idle=%s remaining=%s", idle.idle, idle.remaining)
	}

	active := state.activity(11, 20, now.Add(15*time.Minute), managedSocksIdleTimeout)
	if !active.active {
		t.Fatal("changed flow should be reported as active")
	}
	if active.remaining != managedSocksIdleTimeout {
		t.Fatalf("active flow should report a full timeout, got %s", active.remaining)
	}
	if state.shouldClose(11, 20, now.Add(15*time.Minute), managedSocksIdleTimeout) {
		t.Fatal("flow tracker should accept the changed flow")
	}
	recent := state.activity(11, 20, now.Add(15*time.Minute+30*time.Second), managedSocksIdleTimeout)
	if !recent.active {
		t.Fatal("recently sampled flow should remain active for one sampling window")
	}
	stale := state.activity(11, 20, now.Add(16*time.Minute), managedSocksIdleTimeout)
	if stale.active {
		t.Fatal("flow should become inactive after one sampling window")
	}

	if !state.shouldClose(11, 20, now.Add(45*time.Minute), managedSocksIdleTimeout) {
		t.Fatal("activity query must not reset the idle timer")
	}
}

func TestDurationSecondsCeil(t *testing.T) {
	if got := durationSecondsCeil(1500 * time.Millisecond); got != 2 {
		t.Fatalf("expected duration to round up to 2 seconds, got %d", got)
	}
	if got := durationSecondsCeil(-time.Second); got != 0 {
		t.Fatalf("expected negative duration to clamp to zero, got %d", got)
	}
}

func TestGetManagedSocksActivityReportsRuntimeAndCountdown(t *testing.T) {
	db := &file.DbUtils{JsonDb: file.NewJsonDb(t.TempDir())}
	client := &file.Client{Id: 7, Remark: "test-client"}
	task := &file.Tunnel{
		Id:     client.Id,
		Mode:   file.TaskModeSocks,
		Status: true,
		Client: client,
		Flow:   new(file.Flow),
	}
	db.JsonDb.Clients.Store(client.Id, client)
	db.JsonDb.Tasks.Store(file.TaskMapKey(task), task)

	now := time.Unix(1800000000, 0)
	key := file.TaskMapKey(task)
	RunList.Store(key, nil)
	socksIdleStates.Store(key, &socksIdleState{lastActive: now.Add(-10 * time.Minute)})
	defer RunList.Delete(key)
	defer socksIdleStates.Delete(key)

	status, err := getManagedSocksActivity(db, task.Id, now)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || !status.Running || status.Active || !status.Countdown {
		t.Fatalf("unexpected runtime flags: %+v", status)
	}
	if status.IdleSeconds != 600 || status.RemainingSeconds != 1200 {
		t.Fatalf("unexpected countdown: %+v", status)
	}
	if status.AutoCloseAt != now.Add(20*time.Minute).Unix() {
		t.Fatalf("unexpected auto-close time: %+v", status)
	}

	task.Flow.Add(1, 0)
	status, err = getManagedSocksActivity(db, task.Id, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !status.Active || status.Countdown || status.RemainingSeconds != 1800 {
		t.Fatalf("expected changed flow to be active: %+v", status)
	}

	RunList.Delete(key)
	status, err = getManagedSocksActivity(db, task.Id, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if status.Running || status.Active || status.Countdown || status.RemainingSeconds != 0 {
		t.Fatalf("stopped tunnel must not have an idle countdown: %+v", status)
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
