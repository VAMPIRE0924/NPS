package conn

import (
	"encoding/binary"
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

func TestConfigInfoRejectsMalformedJSON(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()
	go func() {
		_ = binary.Write(clientSide, binary.LittleEndian, int32(1))
		_, _ = clientSide.Write([]byte("{"))
	}()
	if _, err := NewConn(serverSide).GetConfigInfo(); err == nil {
		t.Fatal("expected malformed JSON to fail")
	}
}

func TestTaskAndHostInfoRejectMissingRequiredObjects(t *testing.T) {
	for _, test := range []struct {
		name string
		read func(*Conn) error
	}{
		{name: "task", read: func(c *Conn) error { _, err := c.GetTaskInfo(); return err }},
		{name: "host", read: func(c *Conn) error { _, err := c.GetHostInfo(); return err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			serverSide, clientSide := net.Pipe()
			defer serverSide.Close()
			defer clientSide.Close()
			go func() {
				_ = binary.Write(clientSide, binary.LittleEndian, int32(2))
				_, _ = clientSide.Write([]byte("{}"))
			}()
			if err := test.read(NewConn(serverSide)); err == nil {
				t.Fatal("expected missing target to fail")
			}
		})
	}
}
