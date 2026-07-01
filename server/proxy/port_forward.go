package proxy

import (
	"errors"
	"strings"
	"sync"

	"ehang.io/nps/lib/file"
)

type PortForwardModeServer struct {
	tcp       *TunnelModeServer
	udp       *UdpModeServer
	closeOnce sync.Once
}

func NewPortForwardModeServer(bridge NetBridge, task *file.Tunnel) *PortForwardModeServer {
	return &PortForwardModeServer{
		tcp: NewTunnelModeServer(ProcessTunnel, bridge, task),
		udp: NewUdpModeServer(bridge, task),
	}
}

func (s *PortForwardModeServer) Start() error {
	errCh := make(chan error, 2)
	go func() {
		errCh <- s.tcp.Start()
	}()
	go func() {
		errCh <- s.udp.Start()
	}()

	err := <-errCh
	_ = s.Close()
	if isListenerClosedErr(err) {
		return nil
	}
	return err
}

func (s *PortForwardModeServer) Close() error {
	var errs []string
	closeOne := func(err error) {
		if err != nil && !isListenerClosedErr(err) {
			errs = append(errs, err.Error())
		}
	}
	s.closeOnce.Do(func() {
		if s.tcp != nil {
			closeOne(s.tcp.Close())
		}
		if s.udp != nil {
			closeOne(s.udp.Close())
		}
	})
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func isListenerClosedErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "use of closed network connection")
}
