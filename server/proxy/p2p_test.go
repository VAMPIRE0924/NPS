package proxy

import (
	"net"
	"sync"
	"testing"
	"time"

	"ehang.io/nps/lib/common"
)

func TestP2PStateMatchesBothArrivalOrders(t *testing.T) {
	for _, visitorFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "visitor-first", false: "provider-first"}[visitorFirst], func(t *testing.T) {
			listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			visitor, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
			provider, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
			defer visitor.Close()
			defer provider.Close()
			server := &P2PServer{p2p: make(map[string]*p2p), listener: listener}
			key := "0123456789abcdef0123456789abcdef"
			if visitorFirst {
				server.handleP2P(visitor.LocalAddr().(*net.UDPAddr), string(common.GetWriteStr(key, common.WORK_P2P_VISITOR)))
				server.handleP2P(provider.LocalAddr().(*net.UDPAddr), string(common.GetWriteStr(key, common.WORK_P2P_PROVIDER)))
			} else {
				server.handleP2P(provider.LocalAddr().(*net.UDPAddr), string(common.GetWriteStr(key, common.WORK_P2P_PROVIDER)))
				server.handleP2P(visitor.LocalAddr().(*net.UDPAddr), string(common.GetWriteStr(key, common.WORK_P2P_VISITOR)))
			}
			for conn, want := range map[*net.UDPConn]string{visitor: provider.LocalAddr().String(), provider: visitor.LocalAddr().String()} {
				_ = conn.SetReadDeadline(time.Now().Add(time.Second))
				buf := make([]byte, 128)
				n, _, err := conn.ReadFromUDP(buf)
				if err != nil || string(buf[:n]) != want {
					t.Fatalf("unexpected peer response %q, %v; want %q", buf[:n], err, want)
				}
			}
		})
	}
}

func TestP2PStateConcurrentMalformedInput(t *testing.T) {
	server := &P2PServer{p2p: make(map[string]*p2p)}
	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1234}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			server.handleP2P(addr, "malformed")
		}()
	}
	wg.Wait()
	if len(server.p2p) != 0 {
		t.Fatal("malformed packets allocated P2P state")
	}
}
