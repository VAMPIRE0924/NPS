package controllers

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"ehang.io/nps/lib/common"
	"ehang.io/nps/lib/file"
	"ehang.io/nps/server"
	"github.com/astaxie/beego"
)

type LoginController struct {
	beego.Controller
}

type record struct {
	hasLoginFailTimes int
	lastLoginTime     time.Time
}

type loginAttemptStore struct {
	mu      sync.Mutex
	records map[string]record
}

var loginAttempts = &loginAttemptStore{records: make(map[string]record)}

func (s *loginAttemptStore) blocked(ip string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[ip]
	if !ok {
		return false
	}
	if now.Sub(record.lastLoginTime) >= time.Minute {
		delete(s.records, ip)
		return false
	}
	return record.hasLoginFailTimes >= 10
}

func (s *loginAttemptStore) failure(ip string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record := s.records[ip]
	if now.Sub(record.lastLoginTime) >= time.Minute {
		record.hasLoginFailTimes = 0
	}
	record.hasLoginFailTimes++
	record.lastLoginTime = now
	s.records[ip] = record
}

func (s *loginAttemptStore) success(ip string) {
	s.mu.Lock()
	delete(s.records, ip)
	s.mu.Unlock()
}

func (self *LoginController) Index() {
	webBaseUrl := beego.AppConfig.String("web_base_url")
	self.Data["web_base_url"] = webBaseUrl
	self.Data["xsrf_token"] = self.XSRFToken()
	self.TplName = "login/index.html"
}

func (self *LoginController) Verify() {
	if self.Ctx.Request.Method != http.MethodPost {
		self.Ctx.Output.SetStatus(http.StatusMethodNotAllowed)
		self.Data["json"] = map[string]interface{}{"status": 0, "msg": "method not allowed"}
		self.ServeJSON()
		return
	}
	username := self.GetString("username")
	password := self.GetString("password")
	if self.doLogin(username, password, true) {
		self.Data["json"] = map[string]interface{}{"status": 1, "msg": "login success"}
	} else {
		self.Data["json"] = map[string]interface{}{"status": 0, "msg": "username or password incorrect"}
	}
	self.ServeJSON()
}

func (self *LoginController) doLogin(username, password string, explicit bool) bool {
	now := time.Now()
	ip, _, _ := net.SplitHostPort(self.Ctx.Request.RemoteAddr)
	if loginAttempts.blocked(ip, now) {
		return false
	}
	var auth bool
	if common.ConstantTimeEqual(password, beego.AppConfig.String("web_password")) && common.ConstantTimeEqual(username, beego.AppConfig.String("web_username")) {
		self.SessionRegenerateID()
		self.SetSession("isAdmin", true)
		self.DelSession("clientId")
		self.DelSession("username")
		auth = true
		server.Bridge.Register.Store(common.GetIpByAddr(self.Ctx.Input.IP()), time.Now().Add(time.Hour*time.Duration(2)))
	}
	b, err := beego.AppConfig.Bool("allow_user_login")
	if err == nil && b && !auth {
		file.GetDb().JsonDb.Clients.Range(func(key, value interface{}) bool {
			v := value.(*file.Client)
			if !v.Status || v.NoDisplay {
				return true
			}
			auth = matchesClientWebLogin(v, username, password)
			if auth {
				self.SessionRegenerateID()
				self.SetSession("isAdmin", false)
				self.SetSession("clientId", v.Id)
				self.SetSession("username", "user")
				return false
			}
			return true
		})
	}
	if auth {
		self.SetSession("auth", true)
		loginAttempts.success(ip)
		return true

	}
	if explicit {
		loginAttempts.failure(ip, now)
	}
	return false
}

func matchesClientWebLogin(client *file.Client, username, password string) bool {
	return client != nil && client.Status && !client.NoDisplay && strings.TrimSpace(client.VerifyKey) != "" &&
		common.ConstantTimeEqual(username, "user") && common.ConstantTimeEqual(client.VerifyKey, password)
}

func (self *LoginController) Out() {
	if self.Ctx.Request.Method != http.MethodPost {
		self.Ctx.Output.SetStatus(http.StatusMethodNotAllowed)
		return
	}
	self.DestroySession()
	self.Redirect(beego.AppConfig.String("web_base_url")+"/login/index", 302)
}
