package file

import (
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"

	"ehang.io/nps/lib/common"
	"ehang.io/nps/lib/crypt"
	"ehang.io/nps/lib/rate"
	"github.com/astaxie/beego/logs"
)

type DbUtils struct {
	JsonDb *JsonDb
}

const (
	TaskModePortForward = "portForward"
	TaskModeSocks       = "socks5"
	SocksPortBase       = 10000
)

var (
	Db   *DbUtils
	once sync.Once
)

// init csv from file
func GetDb() *DbUtils {
	once.Do(func() {
		jsonDb := NewJsonDb(common.GetRunPath())
		jsonDb.LoadClientFromJsonFile()
		jsonDb.LoadTaskFromJsonFile()
		jsonDb.LoadHostFromJsonFile()
		Db = &DbUtils{JsonDb: jsonDb}
		Db.EnsureManagedSocks()
	})
	return Db
}

func GetMapKeys(m *sync.Map, isSort bool, sortKey, order string) (keys []int) {
	if sortKey != "" && isSort {
		return sortClientByKey(m, sortKey, order)
	}
	m.Range(func(key, value interface{}) bool {
		keys = append(keys, key.(int))
		return true
	})
	sort.Ints(keys)
	return
}

func GetTaskMapKeys(m *sync.Map) (keys []string) {
	type taskKey struct {
		key  string
		mode string
		id   int
	}
	taskKeys := make([]taskKey, 0)
	m.Range(func(key, value interface{}) bool {
		k, keyOk := key.(string)
		t, taskOk := value.(*Tunnel)
		if keyOk && taskOk {
			taskKeys = append(taskKeys, taskKey{key: k, mode: t.Mode, id: t.Id})
		}
		return true
	})
	sort.Slice(taskKeys, func(i, j int) bool {
		if taskKeys[i].mode != taskKeys[j].mode {
			return taskKeys[i].mode < taskKeys[j].mode
		}
		return taskKeys[i].id < taskKeys[j].id
	})
	keys = make([]string, 0, len(taskKeys))
	for _, taskKey := range taskKeys {
		keys = append(keys, taskKey.key)
	}
	return keys
}

func CanonicalTaskMode(mode string) string {
	switch mode {
	case "tcp", "udp":
		return TaskModePortForward
	default:
		return mode
	}
}

func IsPortForwardMode(mode string) bool {
	return CanonicalTaskMode(mode) == TaskModePortForward
}

func ensureTunnelRuntime(t *Tunnel) {
	if t.Flow == nil {
		t.Flow = new(Flow)
	}
	if t.Target == nil {
		t.Target = new(Target)
	}
}

func ensureHostRuntime(h *Host) {
	if h.Flow == nil {
		h.Flow = new(Flow)
	}
	if h.Target == nil {
		h.Target = new(Target)
	}
}

func ensureClientRuntime(c *Client, resetRate bool) {
	if c.Cnf == nil {
		c.Cnf = new(Config)
	}
	if c.Flow == nil {
		c.Flow = new(Flow)
	}
	if resetRate || c.Rate == nil {
		c.Rate = newClientRate(c.RateLimit)
		c.Rate.Start()
	}
}

func newClientRate(limit int) *rate.Rate {
	if limit > 0 {
		return rate.NewRate(int64(limit * 1024))
	}
	return rate.NewRate(int64(2 << 23))
}

func (s *DbUtils) GetClientList(start, length int, search, sort, order string, clientId int) ([]*Client, int) {
	list := make([]*Client, 0)
	var cnt int
	keys := GetMapKeys(&s.JsonDb.Clients, true, sort, order)
	for _, key := range keys {
		if value, ok := s.JsonDb.Clients.Load(key); ok {
			v := value.(*Client)
			if v.NoDisplay {
				continue
			}
			if clientId != 0 && clientId != v.Id {
				continue
			}
			if search != "" && !(v.Id == common.GetIntNoErrByStr(search) || strings.Contains(v.VerifyKey, search) || strings.Contains(v.Remark, search)) {
				continue
			}
			cnt++
			if start--; start < 0 {
				if length--; length >= 0 {
					list = append(list, v)
				}
			}
		}
	}
	return list, cnt
}

func (s *DbUtils) GetIdByVerifyKey(vKey string, addr string) (id int, err error) {
	var exist bool
	s.JsonDb.Clients.Range(func(key, value interface{}) bool {
		v := value.(*Client)
		if common.Getverifyval(v.VerifyKey) == vKey && v.Status {
			v.Addr = common.GetIpByAddr(addr)
			id = v.Id
			exist = true
			return false
		}
		return true
	})
	if exist {
		return
	}
	return 0, errors.New("not found")
}

func (s *DbUtils) NewTask(t *Tunnel) (err error) {
	if t == nil {
		return errors.New("task is nil")
	}
	if t.Mode == "" {
		return errors.New("task mode is empty")
	}
	t.Mode = CanonicalTaskMode(t.Mode)
	s.JsonDb.idLock.Lock()
	defer s.JsonDb.idLock.Unlock()
	s.JsonDb.Tasks.Range(func(key, value interface{}) bool {
		v := value.(*Tunnel)
		if (v.Mode == "secret" || v.Mode == "p2p") && (t.Mode == "secret" || t.Mode == "p2p") && v.Password == t.Password {
			err = errors.New("secret mode keys must be unique")
			return false
		}
		return true
	})
	if err != nil {
		return
	}
	if t.Mode == TaskModeSocks {
		if err = normalizeManagedSocks(t); err != nil {
			return err
		}
	} else {
		t.Id = nextAvailableTaskId(&s.JsonDb.Tasks, t.Mode)
	}
	if _, ok := s.JsonDb.Tasks.Load(TaskMapKey(t)); ok {
		return errors.New("task id conflict in current mode")
	}
	ensureTunnelRuntime(t)
	s.JsonDb.Tasks.Store(TaskMapKey(t), t)
	s.JsonDb.StoreTasksToJsonFile()
	return
}

func (s *DbUtils) UpdateTaskByModeId(oldMode string, oldId int, t *Tunnel) error {
	if t == nil {
		return errors.New("task is nil")
	}
	if oldMode == "" {
		oldTask, err := s.GetTask(oldId)
		if err != nil {
			return err
		}
		oldMode = oldTask.Mode
	}
	oldMode = CanonicalTaskMode(oldMode)
	t.Mode = CanonicalTaskMode(t.Mode)
	if oldMode == TaskModeSocks || t.Mode == TaskModeSocks {
		return errors.New("socks5 task is managed by client and cannot be modified")
	}
	s.JsonDb.idLock.Lock()
	defer s.JsonDb.idLock.Unlock()
	oldKey := TaskKey(oldMode, oldId)
	if _, ok := s.JsonDb.Tasks.Load(oldKey); !ok {
		return errors.New("not found")
	}
	newKey := TaskMapKey(t)
	if oldKey != newKey {
		if _, ok := s.JsonDb.Tasks.Load(newKey); ok {
			return errors.New("task id conflict in current mode")
		}
		s.JsonDb.Tasks.Delete(oldKey)
	}
	ensureTunnelRuntime(t)
	s.JsonDb.Tasks.Store(newKey, t)
	s.JsonDb.StoreTasksToJsonFile()
	return nil
}

func (s *DbUtils) SetTaskStatusByMode(mode string, id int, status bool) error {
	if mode == "" {
		t, err := s.GetTask(id)
		if err != nil {
			return err
		}
		mode = t.Mode
	}
	mode = CanonicalTaskMode(mode)
	s.JsonDb.idLock.Lock()
	defer s.JsonDb.idLock.Unlock()
	value, ok := s.JsonDb.Tasks.Load(TaskKey(mode, id))
	if !ok {
		return errors.New("not found")
	}
	t := value.(*Tunnel)
	t.Status = status
	if t.Mode == TaskModeSocks {
		if err := normalizeManagedSocks(t); err != nil {
			return err
		}
	}
	s.JsonDb.Tasks.Store(TaskMapKey(t), t)
	s.JsonDb.StoreTasksToJsonFile()
	return nil
}

func (s *DbUtils) DelTaskByMode(mode string, id int) error {
	if mode == "" {
		t, err := s.GetTask(id)
		if err != nil {
			return err
		}
		mode = t.Mode
	}
	mode = CanonicalTaskMode(mode)
	s.JsonDb.idLock.Lock()
	defer s.JsonDb.idLock.Unlock()
	s.JsonDb.Tasks.Delete(TaskKey(mode, id))
	s.JsonDb.StoreTasksToJsonFile()
	return nil
}

// md5 password
func (s *DbUtils) GetTaskByMd5Password(p string) (t *Tunnel) {
	s.JsonDb.Tasks.Range(func(key, value interface{}) bool {
		if crypt.Md5(value.(*Tunnel).Password) == p {
			t = value.(*Tunnel)
			return false
		}
		return true
	})
	return
}

func (s *DbUtils) GetTask(id int) (t *Tunnel, err error) {
	var found []*Tunnel
	s.JsonDb.Tasks.Range(func(key, value interface{}) bool {
		v := value.(*Tunnel)
		if v.Id == id {
			found = append(found, v)
		}
		return true
	})
	if len(found) == 0 {
		return nil, errors.New("not found")
	}
	if len(found) > 1 {
		return nil, errors.New("task id is ambiguous, please specify type")
	}
	return found[0], nil
}

func (s *DbUtils) GetTaskByMode(mode string, id int) (t *Tunnel, err error) {
	if mode == "" {
		return s.GetTask(id)
	}
	mode = CanonicalTaskMode(mode)
	if v, ok := s.JsonDb.Tasks.Load(TaskKey(mode, id)); ok {
		return v.(*Tunnel), nil
	}
	return nil, errors.New("not found")
}

func (s *DbUtils) ResolveTask(mode string, id int) (t *Tunnel, err error) {
	return s.GetTaskByMode(mode, id)
}

func (s *DbUtils) DelHost(id int) error {
	s.JsonDb.idLock.Lock()
	defer s.JsonDb.idLock.Unlock()
	s.JsonDb.Hosts.Delete(id)
	s.JsonDb.StoreHostToJsonFile()
	return nil
}

func (s *DbUtils) IsHostExist(h *Host) bool {
	var exist bool
	s.JsonDb.Hosts.Range(func(key, value interface{}) bool {
		v := value.(*Host)
		if v.Id != h.Id && v.Host == h.Host && h.Location == v.Location && (v.Scheme == "all" || v.Scheme == h.Scheme) {
			exist = true
			return false
		}
		return true
	})
	return exist
}

func (s *DbUtils) NewHost(t *Host) error {
	if t == nil {
		return errors.New("host is nil")
	}
	s.JsonDb.idLock.Lock()
	defer s.JsonDb.idLock.Unlock()
	t.Id = 0
	if t.Location == "" {
		t.Location = "/"
	}
	if s.IsHostExist(t) {
		return errors.New("host has exist")
	}
	t.Id = nextAvailableId(&s.JsonDb.Hosts)
	ensureHostRuntime(t)
	s.JsonDb.Hosts.Store(t.Id, t)
	s.JsonDb.StoreHostToJsonFile()
	return nil
}

func (s *DbUtils) GetHost(start, length int, id int, search string) ([]*Host, int) {
	list := make([]*Host, 0)
	var cnt int
	keys := GetMapKeys(&s.JsonDb.Hosts, false, "", "")
	for _, key := range keys {
		if value, ok := s.JsonDb.Hosts.Load(key); ok {
			v := value.(*Host)
			if search != "" && !(v.Id == common.GetIntNoErrByStr(search) || strings.Contains(v.Host, search) || strings.Contains(v.Remark, search)) {
				continue
			}
			if id == 0 || v.Client.Id == id {
				cnt++
				if start--; start < 0 {
					if length--; length >= 0 {
						list = append(list, v)
					}
				}
			}
		}
	}
	return list, cnt
}

func (s *DbUtils) DelClient(id int) error {
	s.JsonDb.idLock.Lock()
	defer s.JsonDb.idLock.Unlock()
	s.deleteManagedSocksForClientIdLocked(id)
	s.JsonDb.Clients.Delete(id)
	s.JsonDb.StoreTasksToJsonFile()
	s.JsonDb.StoreClientsToJsonFile()
	return nil
}

func (s *DbUtils) NewClient(c *Client) error {
	if c == nil {
		return errors.New("client is nil")
	}
	s.JsonDb.idLock.Lock()
	defer s.JsonDb.idLock.Unlock()
	c.Id = 0
	if c.WebUserName != "" && !s.VerifyUserName(c.WebUserName, 0) {
		return errors.New("web login username duplicate, please reset")
	}

	autoVerifyKey := c.VerifyKey == ""
	for {
		if c.VerifyKey == "" {
			autoVerifyKey = true
			c.VerifyKey = crypt.GetRandomString(16)
		}
		if s.VerifyVkey(c.VerifyKey, 0) {
			break
		}
		if !autoVerifyKey {
			return errors.New("Vkey duplicate, please reset")
		}
		c.VerifyKey = ""
	}

	if c.NoStore && c.NoDisplay {
		c.Id = nextAvailableHiddenId(&s.JsonDb.Clients)
	} else {
		c.Id = nextAvailableId(&s.JsonDb.Clients)
	}
	if _, ok := s.JsonDb.Clients.Load(c.Id); ok {
		return errors.New("client id conflict")
	}
	ensureClientRuntime(c, true)
	s.JsonDb.Clients.Store(c.Id, c)
	if _, err := s.upsertManagedSocksForClientLocked(c); err != nil {
		s.JsonDb.Clients.Delete(c.Id)
		return err
	}
	s.JsonDb.StoreTasksToJsonFile()
	s.JsonDb.StoreClientsToJsonFile()
	return nil
}

func (s *DbUtils) VerifyVkey(vkey string, id int) (res bool) {
	res = true
	s.JsonDb.Clients.Range(func(key, value interface{}) bool {
		v := value.(*Client)
		if v.VerifyKey == vkey && v.Id != id {
			res = false
			return false
		}
		return true
	})
	return res
}

func (s *DbUtils) VerifyUserName(username string, id int) (res bool) {
	res = true
	s.JsonDb.Clients.Range(func(key, value interface{}) bool {
		v := value.(*Client)
		if v.WebUserName == username && v.Id != id {
			res = false
			return false
		}
		return true
	})
	return res
}

func (s *DbUtils) UpdateClient(t *Client) error {
	if t == nil {
		return errors.New("client is nil")
	}
	s.JsonDb.idLock.Lock()
	defer s.JsonDb.idLock.Unlock()
	ensureClientRuntime(t, t.RateLimit == 0 || t.Rate == nil)
	s.JsonDb.Clients.Store(t.Id, t)
	if _, err := s.upsertManagedSocksForClientLocked(t); err != nil {
		return err
	}
	s.JsonDb.StoreTasksToJsonFile()
	s.JsonDb.StoreClientsToJsonFile()
	return nil
}

func (s *DbUtils) UpdateClientBasic(ids []int, username, password string) (int, error) {
	if len(ids) == 0 {
		return 0, errors.New("client id is empty")
	}
	s.JsonDb.idLock.Lock()
	defer s.JsonDb.idLock.Unlock()
	seen := make(map[int]struct{}, len(ids))
	clients := make([]*Client, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return 0, errors.New("client id is invalid")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		value, ok := s.JsonDb.Clients.Load(id)
		if !ok {
			return 0, errors.New("client not found")
		}
		client := value.(*Client)
		if client.NoDisplay {
			return 0, errors.New("client not found")
		}
		seen[id] = struct{}{}
		clients = append(clients, client)
	}
	for _, client := range clients {
		if client.Cnf == nil {
			client.Cnf = new(Config)
		}
		client.Cnf.U = username
		client.Cnf.P = password
	}
	s.JsonDb.StoreClientsToJsonFile()
	return len(clients), nil
}

func (s *DbUtils) IsPubClient(id int) bool {
	client, err := s.GetClient(id)
	if err == nil {
		return client.NoDisplay
	}
	return false
}

func SocksPortByClientId(clientId int) int {
	return SocksPortBase + clientId
}

func ShouldManageClientSocks(c *Client) bool {
	return c != nil && c.Id > 0 && !c.NoStore && !c.NoDisplay
}

func normalizeManagedSocks(t *Tunnel) error {
	if t.Client == nil || t.Client.Id <= 0 {
		return errors.New("socks5 task must bind an existing client")
	}
	t.Mode = TaskModeSocks
	t.Id = t.Client.Id
	t.Port = SocksPortByClientId(t.Client.Id)
	t.Remark = t.Client.Remark
	ensureTunnelRuntime(t)
	return nil
}

func managedSocksNeedsSync(t *Tunnel, c *Client) bool {
	if t == nil || c == nil {
		return true
	}
	return t.Mode != TaskModeSocks ||
		t.Id != c.Id ||
		t.Client == nil ||
		t.Client.Id != c.Id ||
		t.Port != SocksPortByClientId(c.Id) ||
		t.Remark != c.Remark ||
		t.Flow == nil ||
		t.Target == nil
}

func (s *DbUtils) upsertManagedSocksForClientLocked(c *Client) (bool, error) {
	if !ShouldManageClientSocks(c) {
		return s.deleteManagedSocksForClientIdLocked(c.Id), nil
	}
	key := TaskKey(TaskModeSocks, c.Id)
	if value, ok := s.JsonDb.Tasks.Load(key); ok {
		t := value.(*Tunnel)
		if t.Client != nil && t.Client.Id != c.Id {
			return false, errors.New("socks5 task id conflict with another client")
		}
		changed := managedSocksNeedsSync(t, c)
		t.Client = c
		if err := normalizeManagedSocks(t); err != nil {
			return false, err
		}
		s.JsonDb.Tasks.Store(key, t)
		return changed, nil
	}
	t := &Tunnel{
		Id:     c.Id,
		Mode:   TaskModeSocks,
		Client: c,
		Status: false,
	}
	if err := normalizeManagedSocks(t); err != nil {
		return false, err
	}
	s.JsonDb.Tasks.Store(key, t)
	return true, nil
}

func (s *DbUtils) deleteManagedSocksForClientIdLocked(id int) bool {
	key := TaskKey(TaskModeSocks, id)
	if _, ok := s.JsonDb.Tasks.Load(key); ok {
		s.JsonDb.Tasks.Delete(key)
		return true
	}
	return false
}

func (s *DbUtils) EnsureManagedSocks() {
	s.JsonDb.idLock.Lock()
	defer s.JsonDb.idLock.Unlock()
	var changed bool
	s.JsonDb.Clients.Range(func(key, value interface{}) bool {
		c := value.(*Client)
		ok, err := s.upsertManagedSocksForClientLocked(c)
		if err != nil {
			logs.Error("sync managed socks for client %d error: %s", c.Id, err.Error())
			return true
		}
		changed = changed || ok
		return true
	})
	if changed {
		s.JsonDb.StoreTasksToJsonFile()
	}
}

func (s *DbUtils) GetClient(id int) (c *Client, err error) {
	if v, ok := s.JsonDb.Clients.Load(id); ok {
		c = v.(*Client)
		return
	}
	err = errors.New("未找到客户端")
	return
}

func (s *DbUtils) GetClientIdByVkey(vkey string) (id int, err error) {
	var exist bool
	s.JsonDb.Clients.Range(func(key, value interface{}) bool {
		v := value.(*Client)
		if crypt.Md5(v.VerifyKey) == vkey {
			exist = true
			id = v.Id
			return false
		}
		return true
	})
	if exist {
		return
	}
	err = errors.New("未找到客户端")
	return
}

func (s *DbUtils) GetHostById(id int) (h *Host, err error) {
	if v, ok := s.JsonDb.Hosts.Load(id); ok {
		h = v.(*Host)
		return
	}
	err = errors.New("The host could not be parsed")
	return
}

// get key by host from x
func (s *DbUtils) GetInfoByHost(host string, r *http.Request) (h *Host, err error) {
	var hosts []*Host
	//Handling Ported Access
	host = common.GetIpByAddr(host)
	s.JsonDb.Hosts.Range(func(key, value interface{}) bool {
		v := value.(*Host)
		if v.IsClose {
			return true
		}
		//Remove http(s) http(s)://a.proxy.com
		//*.proxy.com *.a.proxy.com  Do some pan-parsing
		if v.Scheme != "all" && v.Scheme != r.URL.Scheme {
			return true
		}
		tmpHost := v.Host
		if strings.Contains(tmpHost, "*") {
			tmpHost = strings.Replace(tmpHost, "*", "", -1)
			if strings.Contains(host, tmpHost) {
				hosts = append(hosts, v)
			}
		} else if v.Host == host {
			hosts = append(hosts, v)
		}
		return true
	})

	for _, v := range hosts {
		//If not set, default matches all
		if v.Location == "" {
			v.Location = "/"
		}
		if strings.Index(r.RequestURI, v.Location) == 0 {
			if h == nil || (len(v.Location) > len(h.Location)) {
				h = v
			}
		}
	}
	if h != nil {
		return
	}
	err = errors.New("The host could not be parsed")
	return
}
