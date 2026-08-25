package controllers

import (
	"strconv"
	"strings"

	"ehang.io/nps/lib/common"
	"ehang.io/nps/lib/file"
	"ehang.io/nps/lib/rate"
	"ehang.io/nps/server"
	"github.com/astaxie/beego"
)

type ClientController struct {
	BaseController
}

func (s *ClientController) List() {
	if s.Ctx.Request.Method == "GET" {
		s.Data["menu"] = "client"
		s.SetInfo("client")
		s.display("client/list")
		return
	}
	s.requirePost()
	start, length := s.GetAjaxParams()
	clientId := 0
	if sessionClientID, ok := s.currentClientID(); ok {
		clientId = sessionClientID
	}
	list, cnt := server.GetClientList(start, length, s.getEscapeString("search"), s.getEscapeString("sort"), s.getEscapeString("order"), clientId)
	cmd := make(map[string]interface{})
	ip := s.Ctx.Request.Host
	cmd["ip"] = common.GetIpByAddr(ip)
	cmd["bridgeType"] = beego.AppConfig.String("bridge_type")
	cmd["bridgePort"] = server.Bridge.TunnelPort
	s.AjaxTable(list, cnt, cnt, cmd)
}

func (s *ClientController) Add() {
	if s.Ctx.Request.Method == "GET" {
		s.Data["menu"] = "client"
		s.SetInfo("add client")
		s.display()
		return
	}
	s.requirePost()
	t := &file.Client{
		VerifyKey: normalizeVerifyKeyInput(s.GetString("vkey")),
		Status:    true,
		Remark:    s.getEscapeString("remark"),
		Cnf: &file.Config{
			U:        s.getEscapeString("u"),
			P:        s.GetString("p"),
			Compress: common.GetBoolByStr(s.getEscapeString("compress")),
			Crypt:    s.GetBoolNoErr("crypt"),
		},
		ConfigConnAllow: s.GetBoolNoErr("config_conn_allow"),
		RateLimit:       s.GetIntNoErr("rate_limit"),
		MaxConn:         s.GetIntNoErr("max_conn"),
		MaxTunnelNum:    s.GetIntNoErr("max_tunnel"),
		Flow: &file.Flow{
			ExportFlow: 0,
			InletFlow:  0,
			FlowLimit:  int64(s.GetIntNoErr("flow_limit")),
		},
	}
	if err := file.GetDb().NewClient(t); err != nil {
		s.AjaxErr(err.Error())
	}
	s.AjaxOk("add success")
}

func (s *ClientController) GetClient() {
	s.requirePost()
	id := s.GetIntNoErr("id")
	data := make(map[string]interface{})
	if c, err := file.GetDb().GetClient(id); err != nil {
		data["code"] = 0
	} else {
		data["code"] = 1
		data["data"] = c
	}
	s.Data["json"] = data
	s.ServeJSON()
}

func (s *ClientController) Basic() {
	s.requirePost()
	if !s.isAdminRequest() {
		s.AjaxErr("permission denied")
	}
	rawIds := strings.TrimSpace(s.getEscapeString("ids"))
	if rawIds == "" {
		rawIds = strings.TrimSpace(s.getEscapeString("id"))
	}
	if rawIds == "" {
		s.AjaxErr("client id is empty")
	}
	ids := make([]int, 0)
	for _, part := range strings.Split(rawIds, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.Atoi(part)
		if err != nil {
			s.AjaxErr("client id is invalid")
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		s.AjaxErr("client id is empty")
	}
	if _, err := file.GetDb().UpdateClientBasic(ids, s.getEscapeString("u"), s.getEscapeString("p")); err != nil {
		s.AjaxErr(err.Error())
	}
	s.AjaxOk("save success")
}

func (s *ClientController) Edit() {
	id := s.GetIntNoErr("id")
	if s.Ctx.Request.Method == "GET" {
		s.Data["menu"] = "client"
		if c, err := file.GetDb().GetClient(id); err != nil {
			s.error()
		} else {
			s.Data["c"] = c
		}
		s.SetInfo("edit client")
		s.display()
		return
	}
	s.requirePost()
	c, err := file.GetDb().GetClient(id)
	if err != nil {
		s.error()
		s.AjaxErr("client ID not found")
		return
	}
	if s.isAdminRequest() {
		verifyKey := normalizeVerifyKeyInput(s.GetString("vkey"))
		if !file.GetDb().VerifyVkey(verifyKey, c.Id) {
			s.AjaxErr("Vkey duplicate, please reset")
			return
		}
		c.VerifyKey = verifyKey
		c.Flow.FlowLimit = int64(s.GetIntNoErr("flow_limit"))
		c.RateLimit = s.GetIntNoErr("rate_limit")
		c.MaxConn = s.GetIntNoErr("max_conn")
		c.MaxTunnelNum = s.GetIntNoErr("max_tunnel")
	}
	c.Remark = s.getEscapeString("remark")
	if c.Cnf == nil {
		c.Cnf = new(file.Config)
	}
	c.Cnf.U = s.getEscapeString("u")
	c.Cnf.P = s.GetString("p")
	c.Cnf.Compress = common.GetBoolByStr(s.getEscapeString("compress"))
	c.Cnf.Crypt = s.GetBoolNoErr("crypt")
	if s.isAdminRequest() {
		c.ConfigConnAllow = s.GetBoolNoErr("config_conn_allow")
	}
	if c.Rate != nil {
		c.Rate.Stop()
	}
	if c.RateLimit > 0 {
		c.Rate = rate.NewRate(int64(c.RateLimit * 1024))
		c.Rate.Start()
	} else {
		c.Rate = rate.NewRate(int64(2 << 23))
		c.Rate.Start()
	}
	if err := file.GetDb().UpdateClient(c); err != nil {
		s.AjaxErr(err.Error())
	}
	s.AjaxOk("save success")
}

func normalizeVerifyKeyInput(value string) string {
	return strings.TrimSpace(value)
}

func (s *ClientController) ChangeStatus() {
	s.requirePost()
	id := s.GetIntNoErr("id")
	if client, err := file.GetDb().GetClient(id); err == nil {
		status := s.GetBoolNoErr("status")
		client.Lock()
		client.Status = status
		client.Unlock()
		if err := file.GetDb().UpdateClient(client); err != nil {
			s.AjaxErr(err.Error())
		}
		if !status {
			server.DelClientConnect(client.Id)
		}
		s.AjaxOk("modified success")
	}
	s.AjaxErr("modified fail")
}

func (s *ClientController) Del() {
	s.requirePost()
	id := s.GetIntNoErr("id")
	_ = server.StopManagedSocksByClientId(id)
	if err := file.GetDb().DelClient(id); err != nil {
		s.AjaxErr("delete error")
	}
	server.DelTunnelAndHostByClientId(id, false)
	server.DelClientConnect(id)
	s.AjaxOk("delete success")
}
