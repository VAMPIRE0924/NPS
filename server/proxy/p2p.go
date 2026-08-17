package proxy

import (
	"net"
	"strings"
	"sync"
	"time"

	"ehang.io/nps/lib/common"
	"github.com/astaxie/beego/logs"
)

type P2PServer struct {
	BaseServer
	p2pPort  int
	p2p      map[string]*p2p
	mu       sync.Mutex
	listener *net.UDPConn
}

type p2p struct {
	visitorAddr  *net.UDPAddr
	providerAddr *net.UDPAddr
	updatedAt    time.Time
}

const (
	maxP2PStates = 4096
	p2pStateTTL  = 30 * time.Second
)

func NewP2PServer(p2pPort int) *P2PServer {
	return &P2PServer{
		p2pPort: p2pPort,
		p2p:     make(map[string]*p2p),
	}
}

func (s *P2PServer) Start() error {
	logs.Info("start p2p server port", s.p2pPort)
	var err error
	s.listener, err = net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: s.p2pPort})
	if err != nil {
		return err
	}
	for {
		buf := common.BufPoolUdp.Get().([]byte)
		n, addr, err := s.listener.ReadFromUDP(buf)
		if err != nil {
			common.BufPoolUdp.Put(buf)
			if strings.Contains(err.Error(), "use of closed network connection") {
				break
			}
			continue
		}
		message := string(buf[:n])
		common.BufPoolUdp.Put(buf)
		s.handleP2P(addr, message)
	}
	return nil
}

func (s *P2PServer) handleP2P(addr *net.UDPAddr, str string) {
	arr := strings.SplitN(str, common.CONN_DATA_SEQ, 3)
	if len(arr) < 2 || len(arr[0]) != 32 || (arr[1] != common.WORK_P2P_VISITOR && arr[1] != common.WORK_P2P_PROVIDER) {
		return
	}
	if len(arr) == 3 && arr[2] != "" {
		return
	}
	if s.listener == nil {
		return
	}
	now := time.Now()
	s.mu.Lock()
	for key, state := range s.p2p {
		if now.Sub(state.updatedAt) > p2pStateTTL {
			delete(s.p2p, key)
		}
	}
	v, ok := s.p2p[arr[0]]
	if !ok {
		if len(s.p2p) >= maxP2PStates {
			s.mu.Unlock()
			return
		}
		v = &p2p{}
		s.p2p[arr[0]] = v
	}
	v.updatedAt = now
	logs.Trace("new p2p connection, role %s, local address %s", arr[1], addr.String())
	addrCopy := *addr
	if arr[1] == common.WORK_P2P_VISITOR {
		v.visitorAddr = &addrCopy
	} else {
		v.providerAddr = &addrCopy
	}
	if v.visitorAddr == nil || v.providerAddr == nil {
		s.mu.Unlock()
		return
	}
	visitorAddr, providerAddr := v.visitorAddr, v.providerAddr
	delete(s.p2p, arr[0])
	s.mu.Unlock()
	_, _ = s.listener.WriteTo([]byte(providerAddr.String()), visitorAddr)
	_, _ = s.listener.WriteTo([]byte(visitorAddr.String()), providerAddr)
}
