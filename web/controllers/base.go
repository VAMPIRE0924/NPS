package controllers

import (
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ehang.io/nps/lib/common"
	"ehang.io/nps/lib/file"
	"ehang.io/nps/server"
	"github.com/astaxie/beego"
)

type BaseController struct {
	beego.Controller
	controllerName   string
	actionName       string
	apiAuthenticated bool
}

// 初始化参数
func (s *BaseController) Prepare() {
	s.Data["web_base_url"] = beego.AppConfig.String("web_base_url")
	controllerName, actionName := s.GetControllerAndAction()
	s.controllerName = strings.ToLower(controllerName[0 : len(controllerName)-10])
	s.actionName = strings.ToLower(actionName)
	if hasAPIAuthHeaders(s.Ctx.Request) {
		if err := requestAPIAuthenticator.verify(s.Ctx.Request, beego.AppConfig.String("auth_key"), time.Now()); err != nil {
			s.rejectUnauthorized(err.Error())
		}
		s.apiAuthenticated = true
		s.EnableXSRF = false
	} else if s.GetString("auth_key") != "" || s.GetString("timestamp") != "" {
		s.rejectUnauthorized("legacy MD5 API authentication is no longer supported")
	} else if s.GetSession("auth") != true {
		if unauthenticatedRequestRequiresAPIAuth(s.Ctx.Request) {
			s.rejectUnauthorized("API authentication required")
		}
		s.Redirect(beego.AppConfig.String("web_base_url")+"/login/index", 302)
		s.StopRun()
	}
	if s.apiAuthenticated {
		s.Data["isAdmin"] = true
	} else if isAdmin, ok := s.GetSession("isAdmin").(bool); !ok {
		s.DestroySession()
		s.rejectUnauthorized("invalid session role")
	} else if !isAdmin {
		clientID, ok := s.GetSession("clientId").(int)
		if !ok || clientID <= 0 {
			s.DestroySession()
			s.rejectUnauthorized("invalid client session")
		}
		s.Ctx.Input.SetData("client_id", clientID)
		s.Ctx.Input.SetParam("client_id", strconv.Itoa(clientID))
		s.Data["isAdmin"] = false
		s.Data["username"] = s.GetSession("username")
		s.CheckUserAuth()
	} else {
		s.Data["isAdmin"] = true
	}
	s.Data["xsrf_token"] = s.XSRFToken()
	s.Data["https_just_proxy"], _ = beego.AppConfig.Bool("https_just_proxy")
	s.Data["allow_user_login"], _ = beego.AppConfig.Bool("allow_user_login")
	s.Data["allow_flow_limit"], _ = beego.AppConfig.Bool("allow_flow_limit")
	s.Data["allow_rate_limit"], _ = beego.AppConfig.Bool("allow_rate_limit")
	s.Data["allow_connection_num_limit"], _ = beego.AppConfig.Bool("allow_connection_num_limit")
	s.Data["allow_multi_ip"], _ = beego.AppConfig.Bool("allow_multi_ip")
	s.Data["system_info_display"], _ = beego.AppConfig.Bool("system_info_display")
	s.Data["allow_tunnel_num_limit"], _ = beego.AppConfig.Bool("allow_tunnel_num_limit")
	s.Data["allow_local_proxy"], _ = beego.AppConfig.Bool("allow_local_proxy")
}

func unauthenticatedRequestRequiresAPIAuth(r *http.Request) bool {
	return r != nil && r.Method == http.MethodPost
}

func (s *BaseController) rejectUnauthorized(message string) {
	s.Ctx.Output.SetStatus(http.StatusUnauthorized)
	s.Data["json"] = ajax(message, 0)
	s.ServeJSON()
	s.StopRun()
}

func (s *BaseController) isAdminRequest() bool {
	if s.apiAuthenticated {
		return true
	}
	isAdmin, _ := s.GetSession("isAdmin").(bool)
	return isAdmin
}

func (s *BaseController) currentClientID() (int, bool) {
	if s.isAdminRequest() {
		return 0, false
	}
	id, ok := s.GetSession("clientId").(int)
	return id, ok && id > 0
}

func (s *BaseController) effectiveClientID(requested int) int {
	if !s.isAdminRequest() {
		id, ok := s.currentClientID()
		if !ok {
			return 0
		}
		return selectEffectiveClientID(false, id, requested)
	}
	return selectEffectiveClientID(true, 0, requested)
}

func selectEffectiveClientID(admin bool, sessionClientID, requested int) int {
	if !admin {
		return sessionClientID
	}
	return requested
}

func clientMayMutateTask(action, mode string) bool {
	mode = file.CanonicalTaskMode(mode)
	switch action {
	case "add", "edit", "del":
		return mode == "secret" || mode == "p2p"
	case "start", "stop":
		return mode == "secret" || mode == "p2p" || mode == file.TaskModeSocks
	default:
		return true
	}
}

func clientMayMutateTaskRequest(action, currentMode, requestedMode string) bool {
	if !clientMayMutateTask(action, currentMode) {
		return false
	}
	if action == "edit" && requestedMode != "" {
		return clientMayMutateTask(action, requestedMode)
	}
	return true
}

func (s *BaseController) rejectForbidden() {
	// Writing only Output.Status during Prepare leaves forbidden GET requests
	// with an empty HTTP 200 response because no later renderer commits the
	// stored status. Commit the status before serializing the same Ajax-shaped
	// error used by POST requests so browsers and API clients both receive an
	// unambiguous denial.
	s.Ctx.Output.Header("Content-Type", "application/json; charset=utf-8")
	s.Ctx.ResponseWriter.WriteHeader(http.StatusForbidden)
	s.Data["json"] = ajax("permission denied", 0)
	s.ServeJSON()
	s.StopRun()
}

func (s *BaseController) requirePost() {
	if s.Ctx.Request.Method != http.MethodPost {
		s.Ctx.Output.SetStatus(http.StatusMethodNotAllowed)
		s.AjaxErr("method not allowed")
	}
}

// 加载模板
func (s *BaseController) display(tpl ...string) {
	s.Data["web_base_url"] = beego.AppConfig.String("web_base_url")
	var tplname string
	if s.Data["menu"] == nil {
		s.Data["menu"] = s.actionName
	}
	if len(tpl) > 0 {
		tplname = strings.Join([]string{tpl[0], "html"}, ".")
	} else {
		tplname = s.controllerName + "/" + s.actionName + ".html"
	}
	ip := s.Ctx.Request.Host
	s.Data["ip"] = common.GetIpByAddr(ip)
	s.Data["bridgeType"] = beego.AppConfig.String("bridge_type")
	if common.IsWindows() {
		s.Data["win"] = ".exe"
	}
	s.Data["p"] = server.Bridge.TunnelPort
	s.Data["proxyPort"] = beego.AppConfig.String("hostPort")
	s.Layout = "public/layout.html"
	s.TplName = tplname
}

// 错误
func (s *BaseController) error() {
	s.Data["web_base_url"] = beego.AppConfig.String("web_base_url")
	s.Layout = "public/layout.html"
	s.TplName = "public/error.html"
}

// getEscapeString
func (s *BaseController) getEscapeString(key string) string {
	return html.EscapeString(s.GetString(key))
}

func (s *BaseController) getTaskMode() string {
	if mode := s.getEscapeString("type"); mode != "" {
		return file.CanonicalTaskMode(mode)
	}
	return file.CanonicalTaskMode(s.getEscapeString("mode"))
}

func authorizationTaskMode(actionName, requestedMode string) string {
	if actionName == "socksstatus" {
		return file.TaskModeSocks
	}
	return requestedMode
}

func authorizationModeForRequest(actionName, requestedMode, oldMode string) string {
	if actionName == "edit" && oldMode != "" {
		return file.CanonicalTaskMode(oldMode)
	}
	return authorizationTaskMode(actionName, requestedMode)
}

func isHostAuthorizationAction(actionName string) bool {
	switch actionName {
	case "hostlist", "gethost", "addhost", "edithost", "delhost":
		return true
	default:
		return false
	}
}

// 去掉没有err返回值的int
func (s *BaseController) GetIntNoErr(key string, def ...int) int {
	strv := s.Ctx.Input.Query(key)
	if len(strv) == 0 && len(def) > 0 {
		return def[0]
	}
	val, _ := strconv.Atoi(strv)
	return val
}

// 获取去掉错误的bool值
func (s *BaseController) GetBoolNoErr(key string, def ...bool) bool {
	strv := s.Ctx.Input.Query(key)
	if len(strv) == 0 && len(def) > 0 {
		return def[0]
	}
	val, _ := strconv.ParseBool(strv)
	return val
}

// ajax正确返回
func (s *BaseController) AjaxOk(str string) {
	s.Data["json"] = ajax(str, 1)
	s.ServeJSON()
	s.StopRun()
}

// ajax错误返回
func (s *BaseController) AjaxErr(str string) {
	s.Data["json"] = ajax(str, 0)
	s.ServeJSON()
	s.StopRun()
}

// 组装ajax
func ajax(str string, status int) map[string]interface{} {
	json := make(map[string]interface{})
	json["status"] = status
	json["msg"] = str
	return json
}

// ajax table返回
func (s *BaseController) AjaxTable(list interface{}, cnt int, recordsTotal int, kwargs map[string]interface{}) {
	json := make(map[string]interface{})
	json["rows"] = list
	json["total"] = recordsTotal
	if kwargs != nil {
		for k, v := range kwargs {
			if v != nil {
				json[k] = v
			}
		}
	}
	s.Data["json"] = json
	s.ServeJSON()
	s.StopRun()
}

// ajax table参数
func (s *BaseController) GetAjaxParams() (start, limit int) {
	return s.GetIntNoErr("offset"), s.GetIntNoErr("limit")
}

func (s *BaseController) SetInfo(name string) {
	s.Data["name"] = name
}

func (s *BaseController) SetType(name string) {
	s.Data["type"] = name
}

func (s *BaseController) CheckUserAuth() {
	clientID, ok := s.currentClientID()
	if !ok {
		s.rejectForbidden()
	}
	if s.controllerName == "client" {
		switch s.actionName {
		case "add", "basic", "changestatus", "del":
			s.rejectForbidden()
		}
		if id := s.GetIntNoErr("id"); id != 0 {
			if id != clientID {
				s.rejectForbidden()
			}
		}
	}
	if s.controllerName == "index" {
		requestedMode := s.getTaskMode()
		mode := authorizationModeForRequest(s.actionName, requestedMode, s.getEscapeString("old_type"))
		if s.Ctx.Request.Method == http.MethodPost {
			switch s.actionName {
			case "add", "edit", "del", "start", "stop":
				if !clientMayMutateTaskRequest(s.actionName, mode, requestedMode) {
					s.rejectForbidden()
				}
			}
		}
		if id := s.GetIntNoErr("id"); id != 0 {
			belong := false
			if isHostAuthorizationAction(s.actionName) {
				if v, ok := file.GetDb().JsonDb.Hosts.Load(id); ok {
					if v.(*file.Host).Client.Id == clientID {
						belong = true
					}
				}
			} else {
				if v, err := file.GetDb().ResolveTask(mode, id); err == nil {
					if v.Client.Id == clientID {
						belong = true
					}
				}
			}
			if !belong {
				s.rejectForbidden()
			}
		}
	}
}
