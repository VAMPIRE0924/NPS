package daemon

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"ehang.io/nps/lib/common"
)

func InitDaemon(f string, runPath string, pidPath string) {
	if len(os.Args) < 2 {
		return
	}
	var args []string
	args = append(args, os.Args[0])
	if len(os.Args) >= 2 {
		args = append(args, os.Args[2:]...)
	}
	args = append(args, "-log=file")
	switch os.Args[1] {
	case "start":
		start(args, f, pidPath, runPath)
		os.Exit(0)
	case "stop":
		stop(f, args[0], pidPath)
		os.Exit(0)
	case "restart":
		stop(f, args[0], pidPath)
		start(args, f, pidPath, runPath)
		os.Exit(0)
	case "status":
		if status(f, pidPath) {
			log.Printf("%s is running", f)
		} else {
			log.Printf("%s is not running", f)
		}
		os.Exit(0)
	case "reload":
		reload(f, pidPath)
		os.Exit(0)
	}
}

func reload(f string, pidPath string) {
	if common.IsWindows() {
		log.Println("reload is not supported on Windows")
		return
	}
	if f == "nps" && !status(f, pidPath) {
		log.Println("reload fail")
		return
	}
	pid, err := readPID(f, pidPath)
	if err != nil {
		log.Println("reload error:", err)
		return
	}
	if exec.Command("kill", "-USR1", strconv.Itoa(pid)).Run() == nil {
		log.Println("reload success")
	} else {
		log.Println("reload fail")
	}
}

func status(f string, pidPath string) bool {
	pid, err := readPID(f, pidPath)
	if err != nil {
		return false
	}
	if !common.IsWindows() {
		return exec.Command("kill", "-0", strconv.Itoa(pid)).Run() == nil
	}
	out, err := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/FO", "CSV", "/NH").Output()
	return err == nil && strings.Contains(string(out), strconv.Itoa(pid))
}

func readPID(f, pidPath string) (int, error) {
	b, err := os.ReadFile(filepath.Join(pidPath, f+".pid"))
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return 0, errors.New("invalid pid file")
	}
	return pid, nil
}

func start(osArgs []string, f string, pidPath, runPath string) {
	if status(f, pidPath) {
		log.Printf(" %s is running", f)
		return
	}
	cmd := exec.Command(osArgs[0], osArgs[1:]...)
	if err := cmd.Start(); err != nil {
		log.Println("start error:", err)
		return
	}
	if cmd.Process.Pid > 0 {
		log.Println("start ok , pid:", cmd.Process.Pid, "config path:", runPath)
		d1 := []byte(strconv.Itoa(cmd.Process.Pid))
		if err := os.WriteFile(filepath.Join(pidPath, f+".pid"), d1, 0600); err != nil {
			log.Println("write pid file error:", err)
		}
	} else {
		log.Println("start error")
	}
}

func stop(f string, _ string, pidPath string) {
	if !status(f, pidPath) {
		log.Printf(" %s is not running", f)
		return
	}
	pid, err := readPID(f, pidPath)
	if err != nil {
		log.Println("stop error:", err)
		return
	}
	var c *exec.Cmd
	if common.IsWindows() {
		c = exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
	} else {
		c = exec.Command("kill", "-9", strconv.Itoa(pid))
	}
	err = c.Run()
	if err != nil {
		log.Println("stop error,", err)
	} else {
		log.Println("stop ok")
	}
}
