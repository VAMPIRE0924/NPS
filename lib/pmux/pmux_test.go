package pmux

import (
	"net"
	"testing"
	"time"
)

func TestPortMuxCloseUnblocksChildListener(t *testing.T) {
	pMux := NewPortMux(0, "manager.example.com")
	if err := pMux.Start(); err != nil {
		t.Fatal(err)
	}
	listener := pMux.GetHttpListener()
	result := make(chan error, 1)
	go func() {
		_, err := listener.Accept()
		result <- err
	}()
	if err := pMux.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected child listener to close with the parent multiplexer")
		}
	case <-time.After(time.Second):
		t.Fatal("child listener did not unblock after multiplexer close")
	}
}

func TestPortMuxStartReturnsBindError(t *testing.T) {
	occupied, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	port := occupied.Addr().(*net.TCPAddr).Port

	pMux := NewPortMux(port, "manager.example.com")
	if err := pMux.Start(); err == nil {
		_ = pMux.Close()
		t.Fatal("expected occupied port to return an error")
	}
}
