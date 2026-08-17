package server

import (
	"ehang.io/nps/lib/version"
	"errors"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ehang.io/nps/bridge"
	"ehang.io/nps/lib/common"
	"ehang.io/nps/lib/file"
	"ehang.io/nps/server/proxy"
	"ehang.io/nps/server/tool"
	"github.com/astaxie/beego"
	"github.com/astaxie/beego/logs"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

var (
	Bridge          *bridge.Bridge
	RunList         sync.Map // map[string]interface{} for tasks, map[int]interface{} for public clients
	flowSessionOnce sync.Once
)

const (
	managedSocksIdleTimeout    = 30 * time.Minute
	managedSocksActivityWindow = time.Minute
)

type socksIdleState struct {
	mu         sync.Mutex
	inletFlow  int64
	exportFlow int64
	lastActive time.Time
	flowActive bool
}

func (s *socksIdleState) shouldClose(inletFlow, exportFlow int64, now time.Time, timeout time.Duration) bool {
	if timeout <= 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastActive.IsZero() {
		s.inletFlow = inletFlow
		s.exportFlow = exportFlow
		s.lastActive = now
		return false
	}
	if s.inletFlow != inletFlow || s.exportFlow != exportFlow {
		s.inletFlow = inletFlow
		s.exportFlow = exportFlow
		s.lastActive = now
		s.flowActive = true
		return false
	}
	return !now.Before(s.lastActive.Add(timeout))
}

type socksActivitySnapshot struct {
	active     bool
	lastActive time.Time
	idle       time.Duration
	remaining  time.Duration
}

func (s *socksIdleState) activity(inletFlow, exportFlow int64, now time.Time, timeout time.Duration) socksActivitySnapshot {
	if timeout <= 0 {
		return socksActivitySnapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.lastActive.IsZero() || s.inletFlow != inletFlow || s.exportFlow != exportFlow {
		return socksActivitySnapshot{
			active:     true,
			lastActive: now,
			remaining:  timeout,
		}
	}
	idle := now.Sub(s.lastActive)
	if idle < 0 {
		idle = 0
	}
	remaining := timeout - idle
	if remaining < 0 {
		remaining = 0
	}
	return socksActivitySnapshot{
		active:     s.flowActive && idle < managedSocksActivityWindow,
		lastActive: s.lastActive,
		idle:       idle,
		remaining:  remaining,
	}
}

// ManagedSocksActivity describes the runtime and idle-close state of a managed
// SOCKS tunnel. Active means its cumulative flow has changed since the latest
// idle tracker sample; querying this value does not reset the idle timer.
type ManagedSocksActivity struct {
	ID                      int   `json:"id"`
	ClientID                int   `json:"client_id"`
	Enabled                 bool  `json:"enabled"`
	Running                 bool  `json:"running"`
	Active                  bool  `json:"active"`
	Countdown               bool  `json:"countdown"`
	LastActiveAt            int64 `json:"last_active_at"`
	IdleSeconds             int64 `json:"idle_seconds"`
	RemainingSeconds        int64 `json:"remaining_seconds"`
	AutoCloseAt             int64 `json:"auto_close_at"`
	AutoCloseTimeoutSeconds int64 `json:"auto_close_timeout_seconds"`
	InletFlow               int64 `json:"inlet_flow"`
	ExportFlow              int64 `json:"export_flow"`
}

func durationSecondsCeil(value time.Duration) int64 {
	if value <= 0 {
		return 0
	}
	return int64((value + time.Second - 1) / time.Second)
}

// GetManagedSocksActivity returns status for the managed SOCKS tunnel whose ID
// is the same as its client ID.
func GetManagedSocksActivity(id int, now time.Time) (*ManagedSocksActivity, error) {
	return getManagedSocksActivity(file.GetDb(), id, now)
}

func getManagedSocksActivity(db *file.DbUtils, id int, now time.Time) (*ManagedSocksActivity, error) {
	t, err := db.GetTaskByMode(file.TaskModeSocks, id)
	if err != nil {
		return nil, err
	}
	t.RLock()
	enabled := t.Status
	clientID := 0
	if t.Client != nil {
		clientID = t.Client.Id
	}
	flow := t.Flow
	t.RUnlock()

	inletFlow, exportFlow := flow.Snapshot()
	result := &ManagedSocksActivity{
		ID:                      t.Id,
		ClientID:                clientID,
		Enabled:                 enabled,
		AutoCloseTimeoutSeconds: int64(managedSocksIdleTimeout / time.Second),
		InletFlow:               inletFlow,
		ExportFlow:              exportFlow,
	}
	taskKey := file.TaskMapKey(t)
	if _, result.Running = RunList.Load(taskKey); !result.Running {
		return result, nil
	}

	stateValue, _ := socksIdleStates.LoadOrStore(taskKey, &socksIdleState{
		inletFlow:  inletFlow,
		exportFlow: exportFlow,
		lastActive: now,
	})
	if _, ok := RunList.Load(taskKey); !ok {
		socksIdleStates.CompareAndDelete(taskKey, stateValue)
		result.Running = false
		return result, nil
	}
	snapshot := stateValue.(*socksIdleState).activity(inletFlow, exportFlow, now, managedSocksIdleTimeout)
	result.Active = snapshot.active
	result.Countdown = !snapshot.active
	result.LastActiveAt = snapshot.lastActive.Unix()
	result.IdleSeconds = int64(snapshot.idle / time.Second)
	result.RemainingSeconds = durationSecondsCeil(snapshot.remaining)
	result.AutoCloseAt = now.Add(snapshot.remaining).Unix()
	return result, nil
}

var socksIdleStates sync.Map

func init() {
	RunList = sync.Map{}
}

// init task from db
func InitFromCsv() {
	//Add a public password
	if vkey := beego.AppConfig.String("public_vkey"); vkey != "" {
		c := file.NewClient(vkey, true, true)
		file.GetDb().NewClient(c)
		RunList.Store(c.Id, nil)
	}
	//Initialize services in server-side files
	file.GetDb().JsonDb.Tasks.Range(func(key, value interface{}) bool {
		if value.(*file.Tunnel).Status {
			AddTask(value.(*file.Tunnel))
		}
		return true
	})
}

// get bridge command
func DealBridgeTask() {
	for {
		select {
		case t := <-Bridge.OpenTask:
			AddTask(t)
		case t := <-Bridge.CloseTask:
			StopServerByMode(t.Mode, t.Id)
		case id := <-Bridge.CloseClient:
			DelTunnelAndHostByClientId(id, true)
			if v, ok := file.GetDb().JsonDb.Clients.Load(id); ok {
				if v.(*file.Client).NoStore {
					file.GetDb().DelClient(id)
				}
			}
		case s := <-Bridge.SecretChan:
			logs.Trace("New secret connection, addr", s.Conn.Conn.RemoteAddr())
			if t := file.GetDb().GetTaskByMd5Password(s.Password); t != nil {
				if t.Status {
					go proxy.NewBaseServer(Bridge, t).DealClient(s.Conn, t.Client, t.Target.TargetStr, nil, common.CONN_TCP, nil, t.Flow, t.Target.LocalProxy)
				} else {
					s.Conn.Close()
					logs.Trace("secret task cannot be processed, status is close")
				}
			} else {
				logs.Trace("secret task cannot be processed")
				s.Conn.Close()
			}
		}
	}
}

// start a new server
func StartNewServer(bridgePort int, cnf *file.Tunnel, bridgeType string, bridgeDisconnect int) {
	Bridge = bridge.NewTunnel(bridgePort, bridgeType, common.GetBoolByStr(beego.AppConfig.String("ip_limit")), &RunList, bridgeDisconnect)
	go func() {
		if err := Bridge.StartTunnel(); err != nil {
			logs.Error("start server bridge error", err)
			os.Exit(0)
		}
	}()
	if p, err := beego.AppConfig.Int("p2p_port"); err == nil {
		go proxy.NewP2PServer(p).Start()
		go proxy.NewP2PServer(p + 1).Start()
		go proxy.NewP2PServer(p + 2).Start()
	}
	go DealBridgeTask()
	go dealClientFlow()
	if svr := NewMode(Bridge, cnf); svr != nil {
		if err := svr.Start(); err != nil {
			logs.Error(err)
		}
		RunList.Store(file.TaskMapKey(cnf), svr)
	} else {
		logs.Error("Incorrect startup mode %s", cnf.Mode)
	}
}

func dealClientFlow() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			dealClientData()
			closeIdleManagedSocks(time.Now())
		}
	}
}

func trackManagedSocksIdle(t *file.Tunnel, now time.Time) {
	if t == nil || t.Mode != file.TaskModeSocks {
		return
	}
	inletFlow, exportFlow := t.Flow.Snapshot()
	socksIdleStates.Store(file.TaskMapKey(t), &socksIdleState{
		inletFlow:  inletFlow,
		exportFlow: exportFlow,
		lastActive: now,
	})
}

func clearManagedSocksIdle(t *file.Tunnel) {
	if t == nil || t.Mode != file.TaskModeSocks {
		return
	}
	socksIdleStates.Delete(file.TaskMapKey(t))
}

func closeIdleManagedSocks(now time.Time) {
	type taskRef struct {
		mode string
		id   int
	}
	var tasks []taskRef
	file.GetDb().JsonDb.Tasks.Range(func(key, value interface{}) bool {
		t := value.(*file.Tunnel)
		if t.Mode != file.TaskModeSocks {
			return true
		}
		taskKey := file.TaskMapKey(t)
		if _, ok := RunList.Load(taskKey); !ok {
			socksIdleStates.Delete(taskKey)
			return true
		}
		inletFlow, exportFlow := t.Flow.Snapshot()
		stateValue, _ := socksIdleStates.LoadOrStore(taskKey, &socksIdleState{
			inletFlow:  inletFlow,
			exportFlow: exportFlow,
			lastActive: now,
		})
		state := stateValue.(*socksIdleState)
		if state.shouldClose(inletFlow, exportFlow, now, managedSocksIdleTimeout) {
			tasks = append(tasks, taskRef{mode: t.Mode, id: t.Id})
		}
		return true
	})
	for _, task := range tasks {
		if err := StopServerByMode(task.mode, task.id); err != nil {
			logs.Warn("auto close idle socks5 mode %s id %d error: %s", task.mode, task.id, err.Error())
		} else {
			logs.Info("auto close idle socks5 mode %s id %d after %s without traffic", task.mode, task.id, managedSocksIdleTimeout)
		}
	}
}

// new a server by mode name
func NewMode(Bridge *bridge.Bridge, c *file.Tunnel) proxy.Service {
	if c == nil {
		return nil
	}
	c.Mode = file.CanonicalTaskMode(c.Mode)
	var service proxy.Service
	switch c.Mode {
	case file.TaskModePortForward:
		service = proxy.NewPortForwardModeServer(Bridge, c)
	case "file":
		service = proxy.NewTunnelModeServer(proxy.ProcessTunnel, Bridge, c)
	case "socks5":
		service = proxy.NewSock5ModeServer(Bridge, c)
	case "httpProxy":
		service = proxy.NewTunnelModeServer(proxy.ProcessHttp, Bridge, c)
	case "tcpTrans":
		service = proxy.NewTunnelModeServer(proxy.HandleTrans, Bridge, c)
	case "webServer":
		InitFromCsv()
		t := &file.Tunnel{
			Port:   0,
			Mode:   "httpHostServer",
			Status: true,
		}
		AddTask(t)
		service = proxy.NewWebServer(Bridge)
	case "httpHostServer":
		httpPort, _ := beego.AppConfig.Int("http_proxy_port")
		httpsPort, _ := beego.AppConfig.Int("https_proxy_port")
		useCache, _ := beego.AppConfig.Bool("http_cache")
		cacheLen, _ := beego.AppConfig.Int("http_cache_length")
		addOrigin, _ := beego.AppConfig.Bool("http_add_origin_header")
		service = proxy.NewHttp(Bridge, c, httpPort, httpsPort, useCache, cacheLen, addOrigin)
	}
	return service
}

// stop server
func StopServerByMode(mode string, id int) error {
	t, err := file.GetDb().ResolveTask(mode, id)
	if err != nil {
		return err
	}
	key := file.TaskMapKey(t)
	defer clearManagedSocksIdle(t)
	if v, ok := RunList.Load(key); ok {
		if svr, ok := v.(proxy.Service); ok {
			if err := svr.Close(); err != nil {
				return err
			}
			logs.Info("stop server mode %s id %d", t.Mode, t.Id)
		} else {
			logs.Warn("stop server mode %s id %d error", t.Mode, t.Id)
		}
		RunList.Delete(key)
	}
	return file.GetDb().SetTaskStatusByMode(t.Mode, t.Id, false)
}

// add task
func AddTask(t *file.Tunnel) error {
	if t == nil {
		return errors.New("task is nil")
	}
	t.Mode = file.CanonicalTaskMode(t.Mode)
	key := file.TaskMapKey(t)
	if _, ok := RunList.Load(key); ok {
		return nil
	}
	if t.Mode == "secret" || t.Mode == "p2p" {
		logs.Info("secret task %s start ", t.Remark)
		RunList.Store(key, nil)
		return nil
	}
	if b := tool.TestServerPort(t.Port, t.Mode); !b && t.Mode != "httpHostServer" {
		logs.Error("taskId %d start error port %d open failed", t.Id, t.Port)
		return errors.New("the port open error")
	}
	if minute, err := beego.AppConfig.Int("flow_store_interval"); err == nil && minute > 0 {
		flowSessionOnce.Do(func() {
			go flowSession(time.Minute * time.Duration(minute))
		})
	}
	if svr := NewMode(Bridge, t); svr != nil {
		logs.Info("tunnel task %s start mode：%s port %d", t.Remark, t.Mode, t.Port)
		RunList.Store(key, svr)
		trackManagedSocksIdle(t, time.Now())
		go func() {
			if err := svr.Start(); err != nil {
				clientId := 0
				if t.Client != nil {
					clientId = t.Client.Id
				}
				logs.Error("clientId %d taskId %d start error %s", clientId, t.Id, err)
				RunList.Delete(key)
				clearManagedSocksIdle(t)
				_ = file.GetDb().SetTaskStatusByMode(t.Mode, t.Id, false)
				return
			}
		}()
	} else {
		return errors.New("the mode is not correct")
	}
	return nil
}

func StartTaskByMode(mode string, id int) error {
	if t, err := file.GetDb().ResolveTask(mode, id); err != nil {
		return err
	} else {
		if err := AddTask(t); err != nil {
			return err
		}
		return file.GetDb().SetTaskStatusByMode(t.Mode, t.Id, true)
	}
}

func StartManagedSocksByClientId(clientId int) error {
	t, err := file.GetDb().GetTaskByMode(file.TaskModeSocks, clientId)
	if err != nil {
		return err
	}
	if _, ok := RunList.Load(file.TaskMapKey(t)); ok {
		return nil
	}
	return StartTaskByMode(file.TaskModeSocks, clientId)
}

func StopManagedSocksByClientId(clientId int) error {
	t, err := file.GetDb().GetTaskByMode(file.TaskModeSocks, clientId)
	if err != nil {
		return nil
	}
	if _, ok := RunList.Load(file.TaskMapKey(t)); !ok {
		return nil
	}
	return StopServerByMode(file.TaskModeSocks, clientId)
}

func DelTaskByMode(mode string, id int) error {
	t, err := file.GetDb().ResolveTask(mode, id)
	if err != nil {
		return err
	}
	if _, ok := RunList.Load(file.TaskMapKey(t)); ok {
		if err := StopServerByMode(t.Mode, t.Id); err != nil {
			return err
		}
	}
	return file.GetDb().DelTaskByMode(t.Mode, t.Id)
}

// get task list by page num
func GetTunnel(start, length int, typeVal string, clientId int, search string) ([]*file.Tunnel, int) {
	typeVal = file.CanonicalTaskMode(typeVal)
	list := make([]*file.Tunnel, 0)
	var cnt int
	keys := file.GetTaskMapKeys(&file.GetDb().JsonDb.Tasks)
	for _, key := range keys {
		if value, ok := file.GetDb().JsonDb.Tasks.Load(key); ok {
			v := value.(*file.Tunnel)
			if typeVal != "" && v.Mode != typeVal {
				continue
			}
			if clientId != 0 && v.Client.Id != clientId {
				continue
			}
			if search != "" && !(v.Id == common.GetIntNoErrByStr(search) || v.Port == common.GetIntNoErrByStr(search) || strings.Contains(v.Password, search) || strings.Contains(v.Remark, search)) {
				continue
			}
			cnt++
			if _, ok := Bridge.Client.Load(v.Client.Id); ok {
				v.Client.Lock()
				v.Client.IsConnect = true
				v.Client.Unlock()
			} else {
				v.Client.Lock()
				v.Client.IsConnect = false
				v.Client.Unlock()
			}
			if start--; start < 0 {
				if length--; length >= 0 {
					if _, ok := RunList.Load(file.TaskMapKey(v)); ok {
						v.Lock()
						v.RunStatus = true
						v.Unlock()
					} else {
						v.Lock()
						v.RunStatus = false
						v.Unlock()
					}
					list = append(list, v)
				}
			}
		}
	}
	return list, cnt
}

// get client list
func GetClientList(start, length int, search, sort, order string, clientId int) (list []*file.Client, cnt int) {
	list, cnt = file.GetDb().GetClientList(start, length, search, sort, order, clientId)
	dealClientData()
	return
}

func dealClientData() {
	file.GetDb().JsonDb.Clients.Range(func(key, value interface{}) bool {
		v := value.(*file.Client)
		if vv, ok := Bridge.Client.Load(v.Id); ok {
			v.Lock()
			v.IsConnect = true
			v.Version = vv.(*bridge.Client).Version
			v.Unlock()
		} else {
			v.Lock()
			v.IsConnect = false
			v.Unlock()
		}
		var inletFlow, exportFlow int64
		file.GetDb().JsonDb.Hosts.Range(func(key, value interface{}) bool {
			h := value.(*file.Host)
			if h.Client.Id == v.Id {
				in, out := h.Flow.Snapshot()
				inletFlow += in
				exportFlow += out
			}
			return true
		})
		file.GetDb().JsonDb.Tasks.Range(func(key, value interface{}) bool {
			t := value.(*file.Tunnel)
			if t.Client.Id == v.Id {
				in, out := t.Flow.Snapshot()
				inletFlow += in
				exportFlow += out
			}
			return true
		})
		v.Flow.Set(inletFlow, exportFlow)
		return true
	})
	return
}

// delete all host and tasks by client id
func DelTunnelAndHostByClientId(clientId int, justDelNoStore bool) {
	type taskRef struct {
		mode string
		id   int
	}
	var tasks []taskRef
	file.GetDb().JsonDb.Tasks.Range(func(key, value interface{}) bool {
		v := value.(*file.Tunnel)
		if justDelNoStore && !v.NoStore {
			return true
		}
		if v.Client.Id == clientId {
			tasks = append(tasks, taskRef{mode: v.Mode, id: v.Id})
		}
		return true
	})
	for _, task := range tasks {
		DelTaskByMode(task.mode, task.id)
	}
	var ids []int
	file.GetDb().JsonDb.Hosts.Range(func(key, value interface{}) bool {
		v := value.(*file.Host)
		if justDelNoStore && !v.NoStore {
			return true
		}
		if v.Client.Id == clientId {
			ids = append(ids, v.Id)
		}
		return true
	})
	for _, id := range ids {
		file.GetDb().DelHost(id)
	}
}

// close the client
func DelClientConnect(clientId int) {
	Bridge.DelClient(clientId)
}

func GetDashboardData() map[string]interface{} {
	data := make(map[string]interface{})
	data["version"] = version.VERSION
	data["hostCount"] = common.GeSynctMapLen(&file.GetDb().JsonDb.Hosts)
	data["clientCount"] = common.GeSynctMapLen(&file.GetDb().JsonDb.Clients)
	if beego.AppConfig.String("public_vkey") != "" { //remove public vkey
		data["clientCount"] = data["clientCount"].(int) - 1
	}
	dealClientData()
	c := 0
	var in, out int64
	file.GetDb().JsonDb.Clients.Range(func(key, value interface{}) bool {
		v := value.(*file.Client)
		if v.IsConnect {
			c += 1
		}
		clientIn, clientOut := v.Flow.Snapshot()
		in += clientIn
		out += clientOut
		return true
	})
	data["clientOnlineCount"] = c
	data["inletFlowCount"] = int(in)
	data["exportFlowCount"] = int(out)
	var portForward, secret, socks5, p2p, http int
	file.GetDb().JsonDb.Tasks.Range(func(key, value interface{}) bool {
		switch value.(*file.Tunnel).Mode {
		case file.TaskModePortForward:
			portForward += 1
		case "socks5":
			socks5 += 1
		case "httpProxy":
			http += 1
		case "p2p":
			p2p += 1
		case "secret":
			secret += 1
		}
		return true
	})

	data["portForwardCount"] = portForward
	data["socks5Count"] = socks5
	data["httpProxyCount"] = http
	data["secretCount"] = secret
	data["p2pCount"] = p2p
	data["bridgeType"] = beego.AppConfig.String("bridge_type")
	data["httpProxyPort"] = beego.AppConfig.String("http_proxy_port")
	data["httpsProxyPort"] = beego.AppConfig.String("https_proxy_port")
	data["ipLimit"] = beego.AppConfig.String("ip_limit")
	data["flowStoreInterval"] = beego.AppConfig.String("flow_store_interval")
	data["serverIp"] = beego.AppConfig.String("p2p_ip")
	data["p2pPort"] = beego.AppConfig.String("p2p_port")
	data["logLevel"] = beego.AppConfig.String("log_level")
	tcpCount := 0

	file.GetDb().JsonDb.Clients.Range(func(key, value interface{}) bool {
		tcpCount += int(atomic.LoadInt32(&value.(*file.Client).NowConn))
		return true
	})
	data["tcpCount"] = tcpCount
	cpuPercet, _ := cpu.Percent(0, true)
	var cpuAll float64
	for _, v := range cpuPercet {
		cpuAll += v
	}
	loads, _ := load.Avg()
	data["load"] = loads.String()
	data["cpu"] = math.Round(cpuAll / float64(len(cpuPercet)))
	swap, _ := mem.SwapMemory()
	data["swap_mem"] = math.Round(swap.UsedPercent)
	vir, _ := mem.VirtualMemory()
	data["virtual_mem"] = math.Round(vir.UsedPercent)
	conn, _ := net.ProtoCounters(nil)
	io1, _ := net.IOCounters(false)
	time.Sleep(time.Millisecond * 500)
	io2, _ := net.IOCounters(false)
	if len(io2) > 0 && len(io1) > 0 {
		data["io_send"] = (io2[0].BytesSent - io1[0].BytesSent) * 2
		data["io_recv"] = (io2[0].BytesRecv - io1[0].BytesRecv) * 2
	}
	for _, v := range conn {
		data[v.Protocol] = v.Stats["CurrEstab"]
	}
	//chart
	var fg int
	if len(tool.ServerStatus) >= 10 {
		fg = len(tool.ServerStatus) / 10
		for i := 0; i <= 9; i++ {
			data["sys"+strconv.Itoa(i+1)] = tool.ServerStatus[i*fg]
		}
	}
	return data
}

// GetClientDashboardData returns the same dashboard shape while limiting all
// account-derived counters and flow totals to one visible client.
func GetClientDashboardData(clientID int) map[string]interface{} {
	data := GetDashboardData()
	client, err := file.GetDb().GetClient(clientID)
	if err != nil || client.NoDisplay {
		data["clientCount"] = 0
		data["clientOnlineCount"] = 0
		data["inletFlowCount"] = 0
		data["exportFlowCount"] = 0
		data["tcpCount"] = 0
	} else {
		in, out := client.Flow.Snapshot()
		data["clientCount"] = 1
		if client.IsConnect {
			data["clientOnlineCount"] = 1
		} else {
			data["clientOnlineCount"] = 0
		}
		data["inletFlowCount"] = int(in)
		data["exportFlowCount"] = int(out)
		data["tcpCount"] = int(atomic.LoadInt32(&client.NowConn))
	}

	counts := map[string]int{
		"portForwardCount": 0,
		"socks5Count":      0,
		"httpProxyCount":   0,
		"secretCount":      0,
		"p2pCount":         0,
	}
	file.GetDb().JsonDb.Tasks.Range(func(_, value interface{}) bool {
		task, ok := value.(*file.Tunnel)
		if !ok || task.Client == nil || task.Client.Id != clientID {
			return true
		}
		switch task.Mode {
		case file.TaskModePortForward:
			counts["portForwardCount"]++
		case file.TaskModeSocks:
			counts["socks5Count"]++
		case "httpProxy":
			counts["httpProxyCount"]++
		case "secret":
			counts["secretCount"]++
		case "p2p":
			counts["p2pCount"]++
		}
		return true
	})
	for key, value := range counts {
		data[key] = value
	}
	hostCount := 0
	file.GetDb().JsonDb.Hosts.Range(func(_, value interface{}) bool {
		host, ok := value.(*file.Host)
		if ok && host.Client != nil && host.Client.Id == clientID {
			hostCount++
		}
		return true
	})
	data["hostCount"] = hostCount
	return data
}

func flowSession(m time.Duration) {
	ticker := time.NewTicker(m)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			file.GetDb().JsonDb.StoreHostToJsonFile()
			file.GetDb().JsonDb.StoreTasksToJsonFile()
			file.GetDb().JsonDb.StoreClientsToJsonFile()
		}
	}
}
