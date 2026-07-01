package goroutine

import (
	"ehang.io/nps/lib/common"
	"ehang.io/nps/lib/file"
	"github.com/panjf2000/ants/v2"
	"io"
	"net"
	"sync"
)

type connGroup struct {
	src      io.ReadWriteCloser
	dst      io.ReadWriteCloser
	wg       *sync.WaitGroup
	flow     *file.Flow
	incoming bool
}

func newConnGroup(dst, src io.ReadWriteCloser, wg *sync.WaitGroup, flow *file.Flow, incoming bool) connGroup {
	return connGroup{
		src:      src,
		dst:      dst,
		wg:       wg,
		flow:     flow,
		incoming: incoming,
	}
}

func copyConnGroup(group interface{}) {
	cg, ok := group.(connGroup)
	if !ok {
		return
	}
	var err error
	_, err = common.CopyBufferWithProgress(cg.dst, cg.src, func(n int64) {
		if cg.flow == nil || n <= 0 {
			return
		}
		if cg.incoming {
			cg.flow.Add(n, 0)
		} else {
			cg.flow.Add(0, n)
		}
	})
	if err != nil {
		cg.src.Close()
		cg.dst.Close()
		//logs.Warn("close npc by copy from nps", err, c.connId)
	}
	cg.wg.Done()
}

type Conns struct {
	conn1 io.ReadWriteCloser // mux connection
	conn2 net.Conn           // outside connection
	flow  *file.Flow
	wg    *sync.WaitGroup
}

func NewConns(c1 io.ReadWriteCloser, c2 net.Conn, flow *file.Flow, wg *sync.WaitGroup) Conns {
	return Conns{
		conn1: c1,
		conn2: c2,
		flow:  flow,
		wg:    wg,
	}
}

func copyConns(group interface{}) {
	conns := group.(Conns)
	wg := new(sync.WaitGroup)
	wg.Add(2)
	// outside to mux : incoming
	_ = connCopyPool.Invoke(newConnGroup(conns.conn1, conns.conn2, wg, conns.flow, true))
	// mux to outside : outgoing
	_ = connCopyPool.Invoke(newConnGroup(conns.conn2, conns.conn1, wg, conns.flow, false))
	wg.Wait()
	conns.wg.Done()
}

var connCopyPool, _ = ants.NewPoolWithFunc(200000, copyConnGroup, ants.WithNonblocking(false))
var CopyConnsPool, _ = ants.NewPoolWithFunc(100000, copyConns, ants.WithNonblocking(false))
