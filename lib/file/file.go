package file

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/astaxie/beego/logs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"ehang.io/nps/lib/common"
	"ehang.io/nps/lib/crypt"
)

func NewJsonDb(runPath string) *JsonDb {
	credentials, err := newCredentialStore(runPath)
	if err != nil {
		panic(err)
	}
	return &JsonDb{
		RunPath:        runPath,
		TaskFilePath:   filepath.Join(runPath, "conf", "tasks.json"),
		HostFilePath:   filepath.Join(runPath, "conf", "hosts.json"),
		ClientFilePath: filepath.Join(runPath, "conf", "clients.json"),
		credentials:    credentials,
	}
}

type JsonDb struct {
	Tasks          sync.Map
	Hosts          sync.Map
	HostsTmp       sync.Map
	Clients        sync.Map
	idLock         sync.Mutex
	RunPath        string
	TaskFilePath   string //task file path
	HostFilePath   string //host file path
	ClientFilePath string //client file path
	credentials    *credentialStore
}

func TaskKey(mode string, id int) string {
	return CanonicalTaskMode(mode) + ":" + strconv.Itoa(id)
}

func TaskMapKey(t *Tunnel) string {
	if t == nil {
		return TaskKey("", 0)
	}
	return TaskKey(t.Mode, t.Id)
}

func (s *JsonDb) LoadTaskFromJsonFile() {
	changed := false
	loadSyncMapFromFile(s.TaskFilePath, func(v string) {
		var err error
		post := new(Tunnel)
		decoded, migrated, decodeErr := s.credentials.decryptJSON([]byte(v))
		if decodeErr != nil {
			panic(fmt.Errorf("decrypt %s: %w", s.TaskFilePath, decodeErr))
		}
		changed = changed || migrated
		if json.Unmarshal(decoded, &post) != nil {
			return
		}
		if post.Client == nil {
			return
		}
		if post.Client, err = s.GetClient(post.Client.Id); err != nil {
			return
		}
		post.Mode = CanonicalTaskMode(post.Mode)
		if post.Mode == TaskModeSocks && ShouldManageClientSocks(post.Client) {
			if err = normalizeManagedSocks(post); err != nil {
				return
			}
		}
		ensureTunnelRuntime(post)
		key := TaskMapKey(post)
		if _, ok := s.Tasks.Load(key); ok {
			oldId := post.Id
			post.Id = nextAvailableTaskId(&s.Tasks, post.Mode)
			key = TaskMapKey(post)
			changed = true
			logs.Warn("duplicate task key %s in task file, reassign to %s:%d", TaskKey(post.Mode, oldId), post.Mode, post.Id)
		}
		s.Tasks.Store(key, post)
	})
	if changed {
		s.StoreTasksToJsonFile()
	}
}

func (s *JsonDb) LoadClientFromJsonFile() {
	changed := false
	loadSyncMapFromFile(s.ClientFilePath, func(v string) {
		post := new(Client)
		decoded, migrated, err := s.credentials.decryptJSON([]byte(v))
		if err != nil {
			panic(fmt.Errorf("decrypt %s: %w", s.ClientFilePath, err))
		}
		changed = changed || migrated
		if bytes.Contains(decoded, []byte(`"WebUserName"`)) || bytes.Contains(decoded, []byte(`"WebPassword"`)) {
			changed = true
		}
		if json.Unmarshal(decoded, &post) != nil {
			return
		}
		if strings.TrimSpace(post.VerifyKey) == "" {
			post.VerifyKey = s.newUniqueVerifyKey()
			changed = true
		}
		ensureClientRuntime(post, true)
		post.NowConn = 0
		s.Clients.Store(post.Id, post)
	})
	if changed {
		s.StoreClientsToJsonFile()
	}
}

func (s *JsonDb) newUniqueVerifyKey() string {
	for {
		candidate := crypt.GetRandomString(16)
		duplicate := false
		s.Clients.Range(func(key, value interface{}) bool {
			if value.(*Client).VerifyKey == candidate {
				duplicate = true
				return false
			}
			return true
		})
		if !duplicate {
			return candidate
		}
	}
}

func (s *JsonDb) LoadHostFromJsonFile() {
	changed := false
	loadSyncMapFromFile(s.HostFilePath, func(v string) {
		var err error
		post := new(Host)
		decoded, migrated, decodeErr := s.credentials.decryptJSON([]byte(v))
		if decodeErr != nil {
			panic(fmt.Errorf("decrypt %s: %w", s.HostFilePath, decodeErr))
		}
		changed = changed || migrated
		if json.Unmarshal(decoded, &post) != nil {
			return
		}
		if post.Client == nil {
			return
		}
		if post.Client, err = s.GetClient(post.Client.Id); err != nil {
			return
		}
		ensureHostRuntime(post)
		s.Hosts.Store(post.Id, post)
	})
	if changed {
		s.StoreHostToJsonFile()
	}
}

func (s *JsonDb) GetClient(id int) (c *Client, err error) {
	if v, ok := s.Clients.Load(id); ok {
		c = v.(*Client)
		return
	}
	err = errors.New("未找到客户端")
	return
}

var hostLock sync.Mutex

func (s *JsonDb) StoreHostToJsonFile() {
	hostLock.Lock()
	storeSyncMapToFile(&s.Hosts, s.HostFilePath, s.credentials)
	hostLock.Unlock()
}

var taskLock sync.Mutex

func (s *JsonDb) StoreTasksToJsonFile() {
	taskLock.Lock()
	storeSyncMapToFile(&s.Tasks, s.TaskFilePath, s.credentials)
	taskLock.Unlock()
}

var clientLock sync.Mutex

func (s *JsonDb) StoreClientsToJsonFile() {
	clientLock.Lock()
	storeSyncMapToFile(&s.Clients, s.ClientFilePath, s.credentials)
	clientLock.Unlock()
}

func nextAvailableHiddenId(m *sync.Map) int {
	used := make(map[int]struct{})
	m.Range(func(key, value interface{}) bool {
		id, ok := key.(int)
		if ok && id < 0 {
			used[id] = struct{}{}
		}
		return true
	})
	for id := -1; ; id-- {
		if _, ok := used[id]; !ok {
			return id
		}
	}
}

func nextAvailableId(m *sync.Map) int {
	used := make(map[int]struct{})
	m.Range(func(key, value interface{}) bool {
		id, ok := key.(int)
		if ok && id > 0 {
			used[id] = struct{}{}
		}
		return true
	})
	for id := 1; ; id++ {
		if _, ok := used[id]; !ok {
			return id
		}
	}
}

func nextAvailableTaskId(m *sync.Map, mode string) int {
	mode = CanonicalTaskMode(mode)
	used := make(map[int]struct{})
	m.Range(func(key, value interface{}) bool {
		t, ok := value.(*Tunnel)
		if ok && t.Id > 0 && (mode == "" || CanonicalTaskMode(t.Mode) == mode) {
			used[t.Id] = struct{}{}
		}
		return true
	})
	for id := 1; ; id++ {
		if _, ok := used[id]; !ok {
			return id
		}
	}
}

func loadSyncMapFromFile(filePath string, f func(value string)) {
	b, err := common.ReadAllFromFile(filePath)
	if err != nil {
		panic(err)
	}
	for _, v := range strings.Split(string(b), "\n"+common.CONN_DATA_SEQ) {
		f(v)
	}
}

func storeSyncMapToFile(m *sync.Map, filePath string, credentials *credentialStore) {
	tmpPath := filePath + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	// first create a temporary file to store
	if err != nil {
		panic(err)
	}
	m.Range(func(key, value interface{}) bool {
		var b []byte
		var err error
		switch value.(type) {
		case *Tunnel:
			obj := value.(*Tunnel)
			if obj.NoStore {
				return true
			}
			b, err = json.Marshal(obj)
		case *Host:
			obj := value.(*Host)
			if obj.NoStore {
				return true
			}
			b, err = json.Marshal(obj)
		case *Client:
			obj := value.(*Client)
			if obj.NoStore {
				return true
			}
			b, err = json.Marshal(obj)
		default:
			return true
		}
		if err != nil {
			return true
		}
		b, err = credentials.encryptJSON(b)
		if err != nil {
			panic(err)
		}
		_, err = file.Write(b)
		if err != nil {
			panic(err)
		}
		_, err = file.Write([]byte("\n" + common.CONN_DATA_SEQ))
		if err != nil {
			panic(err)
		}
		return true
	})
	if err = file.Sync(); err != nil {
		_ = file.Close()
		panic(err)
	}
	if err = file.Close(); err != nil {
		panic(err)
	}
	if err = os.Chmod(tmpPath, 0600); err != nil {
		panic(err)
	}
	// must close file first, then rename it
	err = os.Rename(tmpPath, filePath)
	if err != nil {
		logs.Error(err, "store to file err, data will lost")
	}
	// replace the file, maybe provides atomic operation
}
