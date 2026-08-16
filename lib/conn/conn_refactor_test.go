package conn

import (
	"net"
	"testing"
	"time"

	"ehang.io/nps/lib/file"
)

func TestGetConfigInfoClearsClientReportedBasicAuth(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	sendDone := make(chan error, 1)
	go func() {
		_, err := NewConn(clientSide).SendInfo(&file.Client{
			Cnf: &file.Config{
				U:        "client-user",
				P:        "client-password",
				Compress: true,
				Crypt:    true,
			},
		}, "")
		sendDone <- err
	}()

	client, err := NewConn(serverSide).GetConfigInfo()
	if err != nil {
		t.Fatal(err)
	}
	if client.Cnf == nil {
		t.Fatal("expected config to be initialized")
	}
	if client.Cnf.U != "" || client.Cnf.P != "" {
		t.Fatalf("expected client-reported basic auth to be cleared, got user %q password %q", client.Cnf.U, client.Cnf.P)
	}
	if !client.Cnf.Compress || !client.Cnf.Crypt {
		t.Fatal("expected non-basic client config fields to be preserved")
	}
	if !client.NoStore || !client.Status || client.NoDisplay {
		t.Fatalf("unexpected client flags: NoStore=%v Status=%v NoDisplay=%v", client.NoStore, client.Status, client.NoDisplay)
	}
	if client.Flow == nil {
		t.Fatal("expected flow to be initialized")
	}

	select {
	case err := <-sendDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out sending config")
	}
}
