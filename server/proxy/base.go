package proxy

import (
	"errors"
	"net"
	"net/http"

	"ehang.io/nps/bridge"
	"ehang.io/nps/lib/common"
	"ehang.io/nps/lib/conn"
	"ehang.io/nps/lib/file"
	"github.com/astaxie/beego/logs"
)

type Service interface {
	Start() error
	Close() error
}

type NetBridge interface {
	SendLinkInfo(clientId int, link *conn.Link, t *file.Tunnel) (target net.Conn, err error)
}

// BaseServer struct
type BaseServer struct {
	id           int
	bridge       NetBridge
	task         *file.Tunnel
	errorContent []byte
}

func NewBaseServer(bridge *bridge.Bridge, task *file.Tunnel) *BaseServer {
	return &BaseServer{
		bridge:       bridge,
		task:         task,
		errorContent: nil,
	}
}

// write fail bytes to the connection
func (s *BaseServer) writeConnFail(c net.Conn) {
	c.Write([]byte(common.ConnectionFailBytes))
	c.Write(s.errorContent)
}

// auth check
func (s *BaseServer) auth(r *http.Request, c *conn.Conn, u, p string) error {
	if u != "" && p != "" && !common.CheckAuth(r, u, p) {
		c.Write([]byte(common.UnauthorizedBytes))
		c.Close()
		return errors.New("401 Unauthorized")
	}
	return nil
}

// check flow limit of the client ,and decrease the allow num of client
func (s *BaseServer) CheckFlowAndConnNum(client *file.Client) error {
	inlet, export, limit := client.Flow.SnapshotWithLimit()
	if limit > 0 && (limit<<20) < (export+inlet) {
		return errors.New("Traffic exceeded")
	}
	if !client.GetConn() {
		return errors.New("Connections exceed the current client limit")
	}
	return nil
}

// create a new connection and start bytes copying
func (s *BaseServer) DealClient(c *conn.Conn, client *file.Client, addr string, rb []byte, tp string, f func(), flow *file.Flow, localProxy bool) error {
	link := conn.NewLink(tp, addr, client.Cnf.Crypt, client.Cnf.Compress, c.Conn.RemoteAddr().String(), localProxy)
	if target, err := s.bridge.SendLinkInfo(client.Id, link, s.task); err != nil {
		logs.Warn("get connection from client id %d  error %s", client.Id, err.Error())
		c.Close()
		return err
	} else {
		if f != nil {
			f()
		}
		conn.CopyWaitGroup(target, c.Conn, link.Crypt, link.Compress, client.Rate, flow, true, rb)
	}
	return nil
}
