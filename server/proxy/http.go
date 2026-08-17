package proxy

import (
	"bufio"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ehang.io/nps/bridge"
	"ehang.io/nps/lib/cache"
	"ehang.io/nps/lib/common"
	"ehang.io/nps/lib/conn"
	"ehang.io/nps/lib/file"
	"ehang.io/nps/server/connection"
	"github.com/astaxie/beego/logs"
)

type httpServer struct {
	BaseServer
	httpPort      int
	httpsPort     int
	httpServer    *http.Server
	httpListener  net.Listener
	httpsServer   *http.Server
	httpsListener net.Listener
	useCache      bool
	addOrigin     bool
	cache         *cache.Cache
	cacheLen      int
}

type pendingHTTPResponse struct {
	request   *http.Request
	host      *file.Host
	cacheKey  string
	cacheable bool
	cached    []byte
}

func NewHttp(bridge *bridge.Bridge, c *file.Tunnel, httpPort, httpsPort int, useCache bool, cacheLen int, addOrigin bool) *httpServer {
	httpServer := &httpServer{
		BaseServer: BaseServer{
			task:   c,
			bridge: bridge,
		},
		httpPort:  httpPort,
		httpsPort: httpsPort,
		useCache:  useCache,
		cacheLen:  cacheLen,
		addOrigin: addOrigin,
	}
	if useCache {
		httpServer.cache = cache.New(cacheLen)
	}
	return httpServer
}

func (s *httpServer) Start() error {
	var err error
	if s.errorContent, err = common.ReadAllFromFile(filepath.Join(common.GetRunPath(), "web", "static", "page", "error.html")); err != nil {
		s.errorContent = []byte("nps 404")
	}
	if s.httpPort > 0 {
		s.httpServer = s.NewServer(s.httpPort, "http")
		s.httpListener, err = connection.GetHttpListener()
		if err != nil {
			return err
		}
		go func() {
			if err := s.httpServer.Serve(s.httpListener); err != nil && err != http.ErrServerClosed {
				logs.Error(err)
			}
		}()
	}
	if s.httpsPort > 0 {
		s.httpsServer = s.NewServer(s.httpsPort, "https")
		s.httpsListener, err = connection.GetHttpsListener()
		if err != nil {
			_ = s.Close()
			return err
		}
		go func() {
			if err := NewHttpsServer(s.httpsListener, s.bridge, s.useCache, s.cacheLen).Start(); err != nil {
				logs.Error(err)
			}
		}()
	}
	return nil
}

func (s *httpServer) Close() error {
	if s.httpListener != nil {
		_ = s.httpListener.Close()
	}
	if s.httpsListener != nil {
		s.httpsListener.Close()
	}
	if s.httpsServer != nil {
		s.httpsServer.Close()
	}
	if s.httpServer != nil {
		s.httpServer.Close()
	}
	return nil
}

func (s *httpServer) handleTunneling(w http.ResponseWriter, r *http.Request) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	c, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	s.handleHttp(conn.NewConn(c), r)
}

func (s *httpServer) handleHttp(c *conn.Conn, r *http.Request) {
	var (
		host       *file.Host
		target     net.Conn
		err        error
		connClient io.ReadWriteCloser
		scheme     = r.URL.Scheme
		lk         *conn.Link
		targetAddr string
		lenConn    *conn.LenConn
		isReset    bool
		pending    pendingHTTPResponse
	)
	defer func() {
		if connClient != nil {
			connClient.Close()
		} else {
			s.writeConnFail(c.Conn)
		}
		c.Close()
	}()
reset:
	if isReset {
		host.Client.AddConn()
	}
	if host, err = file.GetDb().GetInfoByHost(r.Host, r); err != nil {
		logs.Notice("the url %s %s %s can't be parsed!", r.URL.Scheme, r.Host, r.RequestURI)
		return
	}
	if err := s.CheckFlowAndConnNum(host.Client); err != nil {
		logs.Warn("client id %d, host id %d, error %s, when https connection", host.Client.Id, host.Id, err.Error())
		return
	}
	if !isReset {
		defer host.Client.AddConn()
	}
	if err = s.auth(r, c, host.Client.Cnf.U, host.Client.Cnf.P); err != nil {
		logs.Warn("auth error", err, r.RemoteAddr)
		return
	}
	if host.Client.Cnf != nil && host.Client.Cnf.U != "" && host.Client.Cnf.P != "" {
		common.StripProxyCredentials(r)
	}
	if targetAddr, err = host.Target.GetRandomTarget(); err != nil {
		logs.Warn(err.Error())
		return
	}
	lk = conn.NewLink("http", targetAddr, host.Client.Cnf.Crypt, host.Client.Cnf.Compress, r.RemoteAddr, host.Target.LocalProxy)
	if target, err = s.bridge.SendLinkInfo(host.Client.Id, lk, nil); err != nil {
		logs.Notice("connect to target %s error %s", lk.Host, err)
		return
	}
	connClient = conn.GetConn(target, lk.Crypt, lk.Compress, host.Client.Rate, true)
	activeConnClient := connClient
	responseRequests := make(chan pendingHTTPResponse)
	responseDone := make(chan struct{})
	// Keep response ordering while using immutable per-request state. This also
	// prevents a keep-alive request for one host from changing the accounting or
	// cache key of an earlier response.
	go func(upstream io.ReadWriteCloser, requests <-chan pendingHTTPResponse) {
		defer close(responseDone)
		defer upstream.Close()
		reader := bufio.NewReader(upstream)
		for pending := range requests {
			if pending.cached != nil {
				n, writeErr := c.Write(pending.cached)
				if writeErr != nil {
					return
				}
				pending.host.Flow.Add(0, int64(n))
				continue
			}
			resp, readErr := http.ReadResponse(reader, pending.request)
			if readErr != nil || resp == nil {
				return
			}
			if pending.cacheable && httpResponseCacheable(resp) {
				b, dumpErr := httputil.DumpResponse(resp, true)
				if dumpErr != nil {
					return
				}
				if _, writeErr := c.Write(b); writeErr != nil {
					return
				}
				pending.host.Flow.Add(0, int64(len(b)))
				s.cache.Add(pending.cacheKey, b)
				continue
			}
			responseConn := conn.NewLenConn(c)
			if writeErr := resp.Write(responseConn); writeErr != nil {
				logs.Error(writeErr)
				return
			}
			pending.host.Flow.Add(0, int64(responseConn.Len))
		}
	}(activeConnClient, responseRequests)

requestLoop:
	for {
		//if the cache start and the request is in the cache list, return the cache
		if s.useCache && httpRequestCacheable(r, host) {
			if v, ok := s.cache.Get(httpCacheKey(host, r)); ok {
				cached, valid := v.([]byte)
				if !valid {
					break requestLoop
				}
				select {
				case responseRequests <- pendingHTTPResponse{host: host, cached: cached}:
				case <-responseDone:
					break requestLoop
				}
				logs.Trace("%s request, method %s, host %s, url %s, remote address %s, return cache", r.URL.Scheme, r.Method, r.Host, r.URL.Path, c.RemoteAddr().String())
				//if return cache and does not create a new conn with client and Connection is not set or close, close the connection.
				if strings.ToLower(r.Header.Get("Connection")) == "close" || strings.ToLower(r.Header.Get("Connection")) == "" {
					break requestLoop
				}
				goto readReq
			}
		}

		//change the host and header and set proxy setting
		common.ChangeHostAndHeader(r, host.HostChange, host.HeaderChange, c.Conn.RemoteAddr().String(), s.addOrigin)
		logs.Trace("%s request, method %s, host %s, url %s, remote address %s, target %s", r.URL.Scheme, r.Method, r.Host, r.URL.Path, c.RemoteAddr().String(), lk.Host)
		pending = pendingHTTPResponse{request: r, host: host}
		pending.cacheable = s.useCache && httpRequestCacheable(r, host)
		if pending.cacheable {
			pending.cacheKey = httpCacheKey(host, r)
		}
		select {
		case responseRequests <- pending:
		case <-responseDone:
			break requestLoop
		}
		//write
		lenConn = conn.NewLenConn(activeConnClient)
		if err := r.Write(lenConn); err != nil {
			logs.Error(err)
			break requestLoop
		}
		host.Flow.Add(int64(lenConn.Len), 0)

	readReq:
		//read req from connection
		_ = c.SetReadDeadline(time.Now().Add(15 * time.Second))
		if r, err = http.ReadRequest(bufio.NewReader(c)); err != nil {
			break requestLoop
		}
		_ = c.SetReadDeadline(time.Time{})
		r.URL.Scheme = scheme
		//What happened ，Why one character less???
		r.Method = resetReqMethod(r.Method)
		if hostTmp, err := file.GetDb().GetInfoByHost(r.Host, r); err != nil {
			logs.Notice("the url %s %s %s can't be parsed!", r.URL.Scheme, r.Host, r.RequestURI)
			break requestLoop
		} else if host != hostTmp {
			host = hostTmp
			isReset = true
			close(responseRequests)
			<-responseDone
			goto reset
		} else {
			if err = s.auth(r, c, host.Client.Cnf.U, host.Client.Cnf.P); err != nil {
				break requestLoop
			}
			if host.Client.Cnf != nil && host.Client.Cnf.U != "" && host.Client.Cnf.P != "" {
				common.StripProxyCredentials(r)
			}
		}
	}
	close(responseRequests)
	<-responseDone
}

func httpCacheKey(host *file.Host, r *http.Request) string {
	path := r.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	return strconv.Itoa(host.Id) + "\x00" + r.URL.Scheme + "\x00" + path + "?" + r.URL.RawQuery
}

func httpRequestCacheable(r *http.Request, host *file.Host) bool {
	if r == nil || r.URL == nil || host == nil || r.Method != http.MethodGet || !strings.Contains(r.URL.Path, ".") {
		return false
	}
	if host.HostChange != "" || host.HeaderChange != "" {
		return false
	}
	if host.Client != nil && host.Client.Cnf != nil && host.Client.Cnf.U != "" && host.Client.Cnf.P != "" {
		return false
	}
	for _, header := range []string{"Authorization", "Proxy-Authorization", "Cookie", "Range"} {
		if r.Header.Get(header) != "" {
			return false
		}
	}
	cacheControl := strings.ToLower(r.Header.Get("Cache-Control"))
	return !strings.Contains(cacheControl, "no-cache") &&
		!strings.Contains(cacheControl, "no-store") &&
		!strings.Contains(cacheControl, "max-age=0") &&
		!strings.EqualFold(strings.TrimSpace(r.Header.Get("Pragma")), "no-cache")
}

func httpResponseCacheable(resp *http.Response) bool {
	if resp == nil || resp.StatusCode != http.StatusOK || resp.Header.Get("Set-Cookie") != "" ||
		resp.Header.Get("Vary") != "" || resp.Header.Get("Content-Range") != "" {
		return false
	}
	cacheControl := strings.ToLower(resp.Header.Get("Cache-Control"))
	return !strings.Contains(cacheControl, "private") &&
		!strings.Contains(cacheControl, "no-cache") &&
		!strings.Contains(cacheControl, "no-store")
}

func resetReqMethod(method string) string {
	if method == "ET" {
		return "GET"
	}
	if method == "OST" {
		return "POST"
	}
	return method
}

func (s *httpServer) NewServer(port int, scheme string) *http.Server {
	return &http.Server{
		Addr:              ":" + strconv.Itoa(port),
		ReadHeaderTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Scheme = scheme
			s.handleTunneling(w, r)
		}),
		// Disable HTTP/2.
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}
}
