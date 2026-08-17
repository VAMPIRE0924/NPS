package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ehang.io/nps/bridge"
	"ehang.io/nps/lib/common"
	"ehang.io/nps/lib/conn"
	"ehang.io/nps/lib/file"
	"ehang.io/nps/server/connection"
	"github.com/astaxie/beego"
	"github.com/astaxie/beego/logs"
)

type TunnelModeServer struct {
	BaseServer
	process  process
	listener net.Listener
}

// tcp|http|host
func NewTunnelModeServer(process process, bridge NetBridge, task *file.Tunnel) *TunnelModeServer {
	s := new(TunnelModeServer)
	s.bridge = bridge
	s.process = process
	s.task = task
	return s
}

// 开始
func (s *TunnelModeServer) Start() error {
	return conn.NewTcpListenerAndProcess(s.task.ServerIp+":"+strconv.Itoa(s.task.Port), func(c net.Conn) {
		if err := s.CheckFlowAndConnNum(s.task.Client); err != nil {
			logs.Warn("client id %d, task id %d,error %s, when tcp connection", s.task.Client.Id, s.task.Id, err.Error())
			c.Close()
			return
		}
		logs.Trace("new tcp connection,local port %d,client %d,remote address %s", s.task.Port, s.task.Client.Id, c.RemoteAddr())
		s.process(conn.NewConn(c), s)
		s.task.Client.AddConn()
	}, &s.listener)
}

// close
func (s *TunnelModeServer) Close() error {
	if s.listener == nil {
		return nil
	}
	return s.listener.Close()
}

// web管理方式
type WebServer struct {
	BaseServer
}

// 开始
func (s *WebServer) Start() error {
	p, _ := beego.AppConfig.Int("web_port")
	if p == 0 {
		stop := make(chan struct{})
		<-stop
	}
	username := beego.AppConfig.String("web_username")
	password := beego.AppConfig.String("web_password")
	if err := validateWebCredentials(username, password); err != nil {
		return err
	}
	if err := validateAPISecret(beego.AppConfig.String("auth_key")); err != nil {
		return err
	}
	beego.BConfig.WebConfig.Session.SessionOn = true
	beego.BConfig.WebConfig.EnableXSRF = true
	webTLS := beego.AppConfig.String("web_open_ssl") == "true"
	beego.BConfig.Listen.EnableHTTPS = webTLS
	xsrfKey := sha256.Sum256([]byte("nps-xsrf:" + username + ":" + password + ":" + beego.AppConfig.String("auth_key")))
	beego.BConfig.WebConfig.XSRFKey = hex.EncodeToString(xsrfKey[:])
	beego.SetStaticPath(beego.AppConfig.String("web_base_url")+"/static", filepath.Join(common.GetRunPath(), "web", "static"))
	beego.SetViewsPath(filepath.Join(common.GetRunPath(), "web", "views"))
	err := errors.New("Web management startup failure ")
	var l net.Listener
	if l, err = connection.GetWebManagerListener(); err == nil {
		beego.InitBeforeHTTPRun()
		webHTTPServer := &http.Server{
			Handler:           newWebSecurityHandler(beego.BeeApp.Handlers, webTLS),
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      60 * time.Second,
			IdleTimeout:       120 * time.Second,
			MaxHeaderBytes:    1 << 20,
		}
		if webTLS {
			keyPath := beego.AppConfig.String("web_key_file")
			certPath := beego.AppConfig.String("web_cert_file")
			err = webHTTPServer.ServeTLS(l, certPath, keyPath)
		} else {
			err = webHTTPServer.Serve(l)
		}
	} else {
		logs.Error(err)
	}
	return err
}

func validateWebCredentials(username, password string) error {
	if strings.TrimSpace(username) == "" {
		return errors.New("web_username must not be empty")
	}
	if len(password) < 12 {
		return errors.New("web_password must contain at least 12 characters")
	}
	weak := map[string]struct{}{
		"123": {}, "admin": {}, "password": {}, "change_me_before_start": {},
	}
	if _, found := weak[strings.ToLower(strings.TrimSpace(password))]; found {
		return errors.New("web_password is a placeholder or commonly used password")
	}
	return nil
}

func validateAPISecret(secret string) error {
	if secret != "" && len(secret) < 32 {
		return errors.New("auth_key must contain at least 32 characters when API authentication is enabled")
	}
	return nil
}

func (s *WebServer) Close() error {
	return nil
}

// new
func NewWebServer(bridge *bridge.Bridge) *WebServer {
	s := new(WebServer)
	s.bridge = bridge
	return s
}

type process func(c *conn.Conn, s *TunnelModeServer) error

// tcp proxy
func ProcessTunnel(c *conn.Conn, s *TunnelModeServer) error {
	targetAddr, err := s.task.Target.GetRandomTarget()
	if err != nil {
		c.Close()
		logs.Warn("tcp port %d ,client id %d,task id %d connect error %s", s.task.Port, s.task.Client.Id, s.task.Id, err.Error())
		return err
	}
	return s.DealClient(c, s.task.Client, targetAddr, nil, common.CONN_TCP, nil, s.task.Flow, s.task.Target.LocalProxy)
}

// http proxy
func ProcessHttp(c *conn.Conn, s *TunnelModeServer) error {
	_, addr, rb, err, r := c.GetHost()
	if err != nil {
		c.Close()
		logs.Info(err)
		return err
	}
	if r.Method == "CONNECT" {
		c.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
		rb = nil
	}
	if err := s.auth(r, c, s.task.Client.Cnf.U, s.task.Client.Cnf.P); err != nil {
		return err
	}
	if s.task.Client.Cnf != nil && s.task.Client.Cnf.U != "" && s.task.Client.Cnf.P != "" {
		common.StripProxyCredentials(r)
	}
	if r.Method != "CONNECT" {
		var sanitized bytes.Buffer
		if err := r.WriteProxy(&sanitized); err != nil {
			return err
		}
		rb = sanitized.Bytes()
	}
	return s.DealClient(c, s.task.Client, addr, rb, common.CONN_TCP, nil, s.task.Flow, s.task.Target.LocalProxy)
}
