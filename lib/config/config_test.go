package config

import (
	"log"
	"regexp"
	"testing"
)

func TestReg(t *testing.T) {
	content := `
[common]
server=127.0.0.1:8284
tp=tcp
vkey=123
[web2]
host=www.baidu.com
host_change=www.sina.com
target=127.0.0.1:8080,127.0.0.1:8082
header_cookkile=122123
header_user-Agent=122123
[web2]
host=www.baidu.com
host_change=www.sina.com
target=127.0.0.1:8080,127.0.0.1:8082
header_cookkile="122123"
header_user-Agent=122123
[tunnel1]
type=udp
target=127.0.0.1:8080
port=9001
compress=snappy
crypt=true
u=1
p=2
[tunnel2]
type=tcp
target=127.0.0.1:8080
port=9001
compress=snappy
crypt=true
u=1
p=2
`
	re, err := regexp.Compile(`\[.+?\]`)
	if err != nil {
		t.Fail()
	}
	log.Println(re.FindAllString(content, -1))
}

func TestDealCommon(t *testing.T) {
	s := `server_addr=127.0.0.1:8284
conn_type=tcp
vkey=123`
	c := dealCommon(s)
	if c.Server != "127.0.0.1:8284" || c.Tp != "tcp" || c.VKey != "123" {
		t.Fatalf("unexpected common config: %#v", c)
	}
	if c.Client == nil || c.Client.Cnf == nil {
		t.Fatal("common config must initialize NPC client runtime fields")
	}
}

func TestGetTitleContent(t *testing.T) {
	s := "[common]"
	if getTitleContent(s) != "common" {
		t.Fail()
	}
}
