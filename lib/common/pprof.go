package common

import (
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/astaxie/beego"
	"github.com/astaxie/beego/logs"
)

func InitPProfFromFile() {
	ip := beego.AppConfig.String("pprof_ip")
	p := beego.AppConfig.String("pprof_port")
	if len(ip) > 0 && len(p) > 0 && IsPort(p) {
		runPProf(ip + ":" + p)
	}
}

func InitPProfFromArg(arg string) {
	if len(arg) > 0 {
		runPProf(arg)
	}
}

func runPProf(ipPort string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	server := &http.Server{
		Addr:              ipPort,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logs.Error("PProf debug server error", err)
		}
	}()
	logs.Info("PProf debug listen on", ipPort)
}
