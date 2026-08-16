package pmux

import (
	"errors"
	"net"
	"sync"
)

type PortListener struct {
	net.Listener
	connCh     chan *PortConn
	addr       net.Addr
	parentDone <-chan struct{}
	done       chan struct{}
	closeOnce  sync.Once
}

func NewPortListener(connCh chan *PortConn, addr net.Addr, parentDone <-chan struct{}) *PortListener {
	return &PortListener{
		connCh:     connCh,
		addr:       addr,
		parentDone: parentDone,
		done:       make(chan struct{}),
	}
}

func (pListener *PortListener) Accept() (net.Conn, error) {
	select {
	case conn := <-pListener.connCh:
		if conn != nil {
			return conn, nil
		}
	case <-pListener.parentDone:
	case <-pListener.done:
	}
	return nil, errors.New("the listener has closed")
}

func (pListener *PortListener) Close() error {
	pListener.closeOnce.Do(func() { close(pListener.done) })
	return nil
}

func (pListener *PortListener) Addr() net.Addr {
	return pListener.addr
}
