package file

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ehang.io/nps/lib/common"
)

func newTestDb(t *testing.T) *DbUtils {
	t.Helper()
	runPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(runPath, "conf"), 0755); err != nil {
		t.Fatal(err)
	}
	jsonDb := NewJsonDb(runPath)
	return &DbUtils{JsonDb: jsonDb}
}

func newTestClient(remark string) *Client {
	return &Client{
		Status: true,
		Remark: remark,
		Cnf:    new(Config),
		Flow:   new(Flow),
	}
}

func newTestTask(mode string, client *Client) *Tunnel {
	return &Tunnel{
		Mode:   mode,
		Client: client,
		Port:   20000,
		Status: true,
		Flow:   new(Flow),
		Target: new(Target),
	}
}

func TestClientIdReuseAndManagedSocks(t *testing.T) {
	db := newTestDb(t)

	for _, remark := range []string{"client-1", "client-2", "client-3"} {
		client := newTestClient(remark)
		client.Id = 99
		if err := db.NewClient(client); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.DelClient(2); err != nil {
		t.Fatal(err)
	}
	reused := newTestClient("client-2-reused")
	reused.Id = 99
	if err := db.NewClient(reused); err != nil {
		t.Fatal(err)
	}
	if reused.Id != 2 {
		t.Fatalf("expected reused client id 2, got %d", reused.Id)
	}

	socks, err := db.GetTaskByMode(TaskModeSocks, reused.Id)
	if err != nil {
		t.Fatal(err)
	}
	if socks.Id != reused.Id || socks.Client.Id != reused.Id {
		t.Fatalf("expected socks id/client id %d, got socks id %d client id %d", reused.Id, socks.Id, socks.Client.Id)
	}
	if socks.Port != SocksPortByClientId(reused.Id) {
		t.Fatalf("expected socks port %d, got %d", SocksPortByClientId(reused.Id), socks.Port)
	}
	if socks.Remark != reused.Remark {
		t.Fatalf("expected socks remark %q, got %q", reused.Remark, socks.Remark)
	}
	if socks.Status {
		t.Fatal("expected new managed socks to be disabled by default")
	}

	if err := db.SetTaskStatusByMode(TaskModeSocks, reused.Id, false); err != nil {
		t.Fatal(err)
	}

	reused.Remark = "client-2-updated"
	if err := db.UpdateClient(reused); err != nil {
		t.Fatal(err)
	}
	socks, err = db.GetTaskByMode(TaskModeSocks, reused.Id)
	if err != nil {
		t.Fatal(err)
	}
	if socks.Remark != reused.Remark {
		t.Fatalf("expected updated socks remark %q, got %q", reused.Remark, socks.Remark)
	}
	if socks.Status {
		t.Fatal("expected client update to preserve disabled socks status")
	}

	if err := db.SetTaskStatusByMode(TaskModeSocks, reused.Id, true); err != nil {
		t.Fatal(err)
	}
	socks, err = db.GetTaskByMode(TaskModeSocks, reused.Id)
	if err != nil {
		t.Fatal(err)
	}
	if !socks.Status {
		t.Fatal("expected socks status to be enabled by status update")
	}

	if err := db.DelClient(reused.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetTaskByMode(TaskModeSocks, reused.Id); err == nil {
		t.Fatal("expected managed socks to be deleted with client")
	}
}

func TestHiddenClientDoesNotConsumeVisibleClientIdPool(t *testing.T) {
	db := newTestDb(t)

	hidden := NewClient("public-vkey", true, true)
	hidden.Id = 99
	if err := db.NewClient(hidden); err != nil {
		t.Fatal(err)
	}
	if hidden.Id >= 0 {
		t.Fatalf("expected hidden client to use private negative id, got %d", hidden.Id)
	}

	visible := newTestClient("visible")
	visible.Id = 99
	if err := db.NewClient(visible); err != nil {
		t.Fatal(err)
	}
	if visible.Id != 1 {
		t.Fatalf("expected first visible client id 1, got %d", visible.Id)
	}
	if _, err := db.GetTaskByMode(TaskModeSocks, visible.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetTaskByMode(TaskModeSocks, hidden.Id); err == nil {
		t.Fatal("expected hidden client not to create managed socks")
	}

	changed, err := db.upsertManagedSocksForClientLocked(visible)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected already-synced managed socks to report no change")
	}

	socks, err := db.GetTaskByMode(TaskModeSocks, visible.Id)
	if err != nil {
		t.Fatal(err)
	}
	socks.Remark = "drifted"
	changed, err = db.upsertManagedSocksForClientLocked(visible)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected drifted managed socks to report a change")
	}
	if socks.Remark != visible.Remark {
		t.Fatalf("expected socks remark to be repaired to %q, got %q", visible.Remark, socks.Remark)
	}
}

func TestPortForwardTaskPoolReusesSmallestAndAcceptsNpcTcpUdpModes(t *testing.T) {
	db := newTestDb(t)
	client := newTestClient("client")
	if err := db.NewClient(client); err != nil {
		t.Fatal(err)
	}

	first := newTestTask("tcp", client)
	second := newTestTask("udp", client)
	first.Id = 99
	second.Id = 99
	if err := db.NewTask(first); err != nil {
		t.Fatal(err)
	}
	if err := db.NewTask(second); err != nil {
		t.Fatal(err)
	}
	if first.Mode != TaskModePortForward || second.Mode != TaskModePortForward {
		t.Fatalf("expected tcp/udp inputs to be stored as %s, got %s and %s", TaskModePortForward, first.Mode, second.Mode)
	}
	if first.Id != 1 || second.Id != 2 {
		t.Fatalf("expected one port-forward pool to allocate ids 1 and 2, got %d and %d", first.Id, second.Id)
	}

	if err := db.DelTaskByMode(TaskModePortForward, 1); err != nil {
		t.Fatal(err)
	}
	reused := newTestTask(TaskModePortForward, client)
	reused.Id = 99
	if err := db.NewTask(reused); err != nil {
		t.Fatal(err)
	}
	if reused.Id != 1 {
		t.Fatalf("expected reused port-forward id 1, got %d", reused.Id)
	}

	next := newTestTask(TaskModePortForward, client)
	next.Id = 99
	if err := db.NewTask(next); err != nil {
		t.Fatal(err)
	}
	if next.Id != 3 {
		t.Fatalf("expected next port-forward id 3, got %d", next.Id)
	}
	if _, err := db.GetTaskByMode(TaskModePortForward, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetTaskByMode(TaskModePortForward, 2); err != nil {
		t.Fatal(err)
	}
}

func TestHostIdReuseSmallestAvailable(t *testing.T) {
	db := newTestDb(t)
	client := newTestClient("client")
	if err := db.NewClient(client); err != nil {
		t.Fatal(err)
	}

	for _, hostName := range []string{"one.example.com", "two.example.com", "three.example.com"} {
		if err := db.NewHost(&Host{
			Id:     99,
			Host:   hostName,
			Client: client,
			Target: new(Target),
			Flow:   new(Flow),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.DelHost(2); err != nil {
		t.Fatal(err)
	}
	host := &Host{
		Id:     99,
		Host:   "reused.example.com",
		Client: client,
		Target: new(Target),
		Flow:   new(Flow),
	}
	if err := db.NewHost(host); err != nil {
		t.Fatal(err)
	}
	if host.Id != 2 {
		t.Fatalf("expected reused host id 2, got %d", host.Id)
	}

	dupe := &Host{
		Id:     1,
		Host:   "one.example.com",
		Client: client,
		Target: new(Target),
		Flow:   new(Flow),
	}
	if err := db.NewHost(dupe); err == nil {
		t.Fatal("expected duplicate host to be rejected even when caller supplies existing id")
	}
}

func TestLoadTaskFromJsonFileReassignsDuplicateTaskId(t *testing.T) {
	db := newTestDb(t)
	client := newTestClient("client")
	client.Id = 1
	db.JsonDb.Clients.Store(client.Id, client)

	first := newTestTask(TaskModePortForward, client)
	second := newTestTask(TaskModePortForward, client)
	first.Id = 1
	second.Id = 1
	first.Remark = "first"
	second.Remark = "second"

	var records []byte
	for _, task := range []*Tunnel{first, second} {
		b, err := json.Marshal(task)
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, b...)
		records = append(records, []byte("\n"+common.CONN_DATA_SEQ)...)
	}
	if err := os.WriteFile(db.JsonDb.TaskFilePath, records, 0644); err != nil {
		t.Fatal(err)
	}

	db.JsonDb.LoadTaskFromJsonFile()
	if _, err := db.GetTaskByMode(TaskModePortForward, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetTaskByMode(TaskModePortForward, 2); err != nil {
		t.Fatal(err)
	}
}
