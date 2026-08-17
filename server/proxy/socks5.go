package proxy

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"ehang.io/nps/lib/common"
	"ehang.io/nps/lib/conn"
	"ehang.io/nps/lib/file"
	"github.com/astaxie/beego/logs"
)

const (
	ipV4            = 1
	domainName      = 3
	ipV6            = 4
	connectMethod   = 1
	bindMethod      = 2
	associateMethod = 3
	// The maximum packet size of any udp Associate packet, based on ethernet's max size,
	// minus the IP and UDP headers. IPv4 has a 20 byte header, UDP adds an
	// additional 4 bytes.  This is a total overhead of 24 bytes.  Ethernet's
	// max packet size is 1500 bytes,  1500 - 24 = 1476.
	maxUDPPacketSize = 1476
)

const (
	succeeded uint8 = iota
	serverFailure
	notAllowed
	networkUnreachable
	hostUnreachable
	connectionRefused
	ttlExpired
	commandNotSupported
	addrTypeNotSupported
)

const (
	UserPassAuth    = uint8(2)
	userAuthVersion = uint8(1)
	authSuccess     = uint8(0)
	authFailure     = uint8(1)
)

type Sock5ModeServer struct {
	BaseServer
	listener net.Listener
}

// req
func (s *Sock5ModeServer) handleRequest(c net.Conn) {
	/*
		The SOCKS request is formed as follows:
		+----+-----+-------+------+----------+----------+
		|VER | CMD |  RSV  | ATYP | DST.ADDR | DST.PORT |
		+----+-----+-------+------+----------+----------+
		| 1  |  1  | X'00' |  1   | Variable |    2     |
		+----+-----+-------+------+----------+----------+
	*/
	header := make([]byte, 3)

	_, err := io.ReadFull(c, header)

	if err != nil {
		logs.Warn("illegal request", err)
		c.Close()
		return
	}

	switch header[1] {
	case connectMethod:
		s.handleConnect(c)
	case bindMethod:
		s.handleBind(c)
	case associateMethod:
		s.handleUDP(c)
	default:
		s.sendReply(c, commandNotSupported)
		c.Close()
	}
}

// reply
func (s *Sock5ModeServer) sendReply(c net.Conn, rep uint8) {
	reply := []byte{
		5,
		rep,
		0,
		1,
	}

	localHost, localPort, err := net.SplitHostPort(c.LocalAddr().String())
	if err != nil {
		localHost, localPort = "0.0.0.0", "0"
	}
	ipBytes := net.ParseIP(localHost).To4()
	if ipBytes == nil {
		ipBytes = net.IPv4zero
	}
	nPort, _ := strconv.ParseUint(localPort, 10, 16)
	reply = append(reply, ipBytes...)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(nPort))
	reply = append(reply, portBytes...)

	_, _ = c.Write(reply)
}

func readSocksAddress(c io.Reader) (string, uint16, error) {
	addrType := make([]byte, 1)
	if _, err := io.ReadFull(c, addrType); err != nil {
		return "", 0, err
	}
	var host string
	switch addrType[0] {
	case ipV4:
		ipv4 := make(net.IP, net.IPv4len)
		if _, err := io.ReadFull(c, ipv4); err != nil {
			return "", 0, err
		}
		host = ipv4.String()
	case ipV6:
		ipv6 := make(net.IP, net.IPv6len)
		if _, err := io.ReadFull(c, ipv6); err != nil {
			return "", 0, err
		}
		host = ipv6.String()
	case domainName:
		var domainLen uint8
		if err := binary.Read(c, binary.BigEndian, &domainLen); err != nil || domainLen == 0 {
			return "", 0, errors.New("invalid SOCKS domain length")
		}
		domain := make([]byte, domainLen)
		if _, err := io.ReadFull(c, domain); err != nil {
			return "", 0, err
		}
		host = string(domain)
	default:
		return "", 0, errors.New("unsupported SOCKS address type")
	}

	var port uint16
	if err := binary.Read(c, binary.BigEndian, &port); err != nil {
		return "", 0, err
	}
	return host, port, nil
}

// do conn
func (s *Sock5ModeServer) doConnect(c net.Conn, command uint8) {
	host, port, err := readSocksAddress(c)
	if err != nil {
		s.sendReply(c, addrTypeNotSupported)
		_ = c.Close()
		return
	}
	_ = c.SetDeadline(time.Time{})
	// connect to host
	addr := net.JoinHostPort(host, strconv.Itoa(int(port)))
	var ltype string
	if command == associateMethod {
		ltype = common.CONN_UDP
	} else {
		ltype = common.CONN_TCP
	}
	s.DealClient(conn.NewConn(c), s.task.Client, addr, nil, ltype, func() {
		s.sendReply(c, succeeded)
	}, s.task.Flow, s.task.Target.LocalProxy)
	return
}

// conn
func (s *Sock5ModeServer) handleConnect(c net.Conn) {
	s.doConnect(c, connectMethod)
}

// passive mode
func (s *Sock5ModeServer) handleBind(c net.Conn) {
	s.sendReply(c, commandNotSupported)
	_ = c.Close()
}
func (s *Sock5ModeServer) sendUdpReply(writeConn net.Conn, c net.Conn, rep uint8, serverIp string) {
	reply := []byte{
		5,
		rep,
		0,
		1,
	}
	_, localPort, err := net.SplitHostPort(c.LocalAddr().String())
	if err != nil {
		localPort = "0"
	}
	localHost := serverIp
	ipBytes := net.ParseIP(localHost).To4()
	if ipBytes == nil {
		ipBytes = net.IPv4zero
	}
	nPort, _ := strconv.ParseUint(localPort, 10, 16)
	reply = append(reply, ipBytes...)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(nPort))
	reply = append(reply, portBytes...)
	_, _ = writeConn.Write(reply)

}

func (s *Sock5ModeServer) handleUDP(c net.Conn) {
	defer c.Close()
	host, port, err := readSocksAddress(c)
	if err != nil {
		s.sendReply(c, addrTypeNotSupported)
		return
	}
	_ = c.SetDeadline(time.Time{})
	logs.Warn(host, strconv.Itoa(int(port)))
	replyAddr, err := net.ResolveUDPAddr("udp", s.task.ServerIp+":0")
	if err != nil {
		logs.Error("build local reply addr error", err)
		return
	}
	reply, err := net.ListenUDP("udp", replyAddr)
	if err != nil {
		s.sendReply(c, addrTypeNotSupported)
		logs.Error("listen local reply udp port error")
		return
	}
	// reply the local addr
	remoteHost, _, splitErr := net.SplitHostPort(c.RemoteAddr().String())
	if splitErr != nil {
		remoteHost = c.RemoteAddr().String()
	}
	s.sendUdpReply(c, reply, succeeded, common.GetServerIpByClientIp(net.ParseIP(remoteHost)))
	defer reply.Close()
	// new a tunnel to client
	link := conn.NewLink("udp5", "", s.task.Client.Cnf.Crypt, s.task.Client.Cnf.Compress, c.RemoteAddr().String(), false)
	target, err := s.bridge.SendLinkInfo(s.task.Client.Id, link, s.task)
	if err != nil {
		logs.Warn("get connection from client id %d  error %s", s.task.Client.Id, err.Error())
		return
	}

	var clientAddr atomic.Pointer[net.UDPAddr]
	// copy buffer
	go func() {
		b := common.BufPoolUdp.Get().([]byte)
		defer common.BufPoolUdp.Put(b)
		defer c.Close()

		for {
			n, laddr, err := reply.ReadFromUDP(b)
			if err != nil {
				logs.Error("read data from %s err %s", reply.LocalAddr().String(), err.Error())
				return
			}
			if clientAddr.Load() == nil {
				clientAddr.Store(laddr)
			}
			if _, err := target.Write(b[:n]); err != nil {
				logs.Error("write data to client error", err.Error())
				return
			}
			s.task.Flow.Add(int64(n), 0)
		}
	}()

	go func() {
		var l int32
		b := common.BufPoolUdp.Get().([]byte)
		defer common.BufPoolUdp.Put(b)
		defer c.Close()
		for {
			if err := binary.Read(target, binary.LittleEndian, &l); err != nil {
				logs.Warn("read len bytes error", err.Error())
				return
			}
			if l >= common.PoolSizeUdp || l <= 0 {
				logs.Warn("invalid udp payload length", l)
				return
			}
			if _, err := io.ReadFull(target, b[:int(l)]); err != nil {
				logs.Warn("read data form client error", err.Error())
				return
			}
			remoteAddr := clientAddr.Load()
			if remoteAddr == nil {
				continue
			}
			if _, err := reply.WriteToUDP(b[:int(l)], remoteAddr); err != nil {
				logs.Warn("write data to user ", err.Error())
				return
			}
			s.task.Flow.Add(0, int64(l))
		}
	}()

	b := common.BufPoolUdp.Get().([]byte)
	defer common.BufPoolUdp.Put(b)
	defer target.Close()
	for {
		_, err := c.Read(b)
		if err != nil {
			c.Close()
			return
		}
	}
}

// new conn
func (s *Sock5ModeServer) handleConn(c net.Conn) {
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	buf := make([]byte, 2)
	if _, err := io.ReadFull(c, buf); err != nil {
		logs.Warn("negotiation err", err)
		c.Close()
		return
	}

	if version := buf[0]; version != 5 {
		logs.Warn("only support socks5, request from: ", c.RemoteAddr())
		c.Close()
		return
	}
	nMethods := buf[1]
	if nMethods == 0 {
		_, _ = c.Write([]byte{5, 0xff})
		_ = c.Close()
		return
	}

	methods := make([]byte, nMethods)
	if _, err := io.ReadFull(c, methods); err != nil {
		logs.Warn("wrong method")
		c.Close()
		return
	}
	requiresPassword := (s.task.Client.Cnf.U != "" && s.task.Client.Cnf.P != "") || (s.task.MultiAccount != nil && len(s.task.MultiAccount.AccountMap) > 0)
	selected := byte(0)
	if requiresPassword {
		selected = UserPassAuth
	}
	offered := false
	for _, method := range methods {
		if method == selected {
			offered = true
			break
		}
	}
	if !offered {
		_, _ = c.Write([]byte{5, 0xff})
		_ = c.Close()
		return
	}
	buf[1] = selected
	if _, err := c.Write(buf); err != nil {
		_ = c.Close()
		return
	}
	if requiresPassword {
		if err := s.Auth(c); err != nil {
			c.Close()
			logs.Warn("Validation failed:", err)
			return
		}
	}
	s.handleRequest(c)
}

// socks5 auth
func (s *Sock5ModeServer) Auth(c net.Conn) error {
	header := []byte{0, 0}
	if _, err := io.ReadFull(c, header); err != nil {
		return err
	}
	if header[0] != userAuthVersion {
		return errors.New("验证方式不被支持")
	}
	userLen := int(header[1])
	user := make([]byte, userLen)
	if _, err := io.ReadFull(c, user); err != nil {
		return err
	}
	if _, err := io.ReadFull(c, header[:1]); err != nil {
		return errors.New("密码长度获取错误")
	}
	passLen := int(header[0])
	pass := make([]byte, passLen)
	if _, err := io.ReadFull(c, pass); err != nil {
		return err
	}

	var U, P string
	if s.task.MultiAccount != nil {
		// enable multi user auth
		U = string(user)
		var ok bool
		P, ok = s.task.MultiAccount.AccountMap[U]
		if !ok {
			return errors.New("验证不通过")
		}
	} else {
		U = s.task.Client.Cnf.U
		P = s.task.Client.Cnf.P
	}

	if string(user) == U && string(pass) == P {
		if _, err := c.Write([]byte{userAuthVersion, authSuccess}); err != nil {
			return err
		}
		return nil
	} else {
		if _, err := c.Write([]byte{userAuthVersion, authFailure}); err != nil {
			return err
		}
		return errors.New("验证不通过")
	}
}

// start
func (s *Sock5ModeServer) Start() error {
	return conn.NewTcpListenerAndProcess(s.task.ServerIp+":"+strconv.Itoa(s.task.Port), func(c net.Conn) {
		if err := s.CheckFlowAndConnNum(s.task.Client); err != nil {
			logs.Warn("client id %d, task id %d, error %s, when socks5 connection", s.task.Client.Id, s.task.Id, err.Error())
			c.Close()
			return
		}
		logs.Trace("New socks5 connection,client %d,remote address %s", s.task.Client.Id, c.RemoteAddr())
		s.handleConn(c)
		s.task.Client.AddConn()
	}, &s.listener)
}

// new
func NewSock5ModeServer(bridge NetBridge, task *file.Tunnel) *Sock5ModeServer {
	s := new(Sock5ModeServer)
	s.bridge = bridge
	s.task = task
	return s
}

// close
func (s *Sock5ModeServer) Close() error {
	if s.listener == nil {
		return nil
	}
	return s.listener.Close()
}
