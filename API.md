# NPS API

## 基础约定

接口路径受 `nps.conf` 中的 `web_base_url` 影响：

```text
web_base_url 为空:  /client/list/
web_base_url=/nps: /nps/client/list/
```

推荐请求方式：

```text
POST
Content-Type: application/x-www-form-urlencoded
```

通用响应：

```json
{"status":1,"msg":"success message"}
{"status":0,"msg":"error message"}
{"rows":[],"total":0}
{"code":1,"data":{}}
{"code":0}
```

## 鉴权

保留 NPS 原有 `auth_key` 机制：

```text
timestamp = 当前 Unix 秒级时间戳
auth_key  = md5(nps.conf 中的 auth_key + timestamp)
```

服务端允许约 20 秒时间偏差。

示例：

```bash
curl -X POST "http://127.0.0.1:8080/index/start/" \
  -d "id=1" \
  -d "type=socks5" \
  -d "timestamp=<timestamp>" \
  -d "auth_key=<md5>"
```

## ID 规则

新增对象时由 NPS 服务端分配当前池内最小可用正整数 ID。调用方不要自己计算 `max(id)+1`。

独立 ID 池：

```text
Client.Id
Host.Id
portForward.Id
socks5.Id
httpProxy.Id
secret.Id
p2p.Id
file.Id
```

任务内部唯一键：

```text
mode:id
```

例如：

```text
portForward:1
socks5:1
httpProxy:1
```

调用任务接口时必须传 `type`。

## 客户端接口

### 客户端列表

```text
POST /client/list/
```

参数：

```text
offset
limit
search
sort
order
timestamp
auth_key
```

### 获取单个客户端

```text
POST /client/getclient/
```

参数：

```text
id
timestamp
auth_key
```

### 新增客户端

```text
POST /client/add/
```

参数：

```text
remark
vkey                留空则自动生成
u                   Basic 用户名，服务端配置
p                   Basic 密码，服务端配置
compress            1/0
crypt               1/0
config_conn_allow   1/0
rate_limit          KB/s，0 表示不限制
flow_limit          MB，0 表示不限制
max_conn            0 表示不限制
max_tunnel          0 表示不限制
web_username
web_password
timestamp
auth_key
```

行为：

```text
1. Client.Id 自动取最小可用正整数。
2. 自动创建托管 SOCKS5。
3. socks5.Id = Client.Id。
4. socks5.Port = 10000 + Client.Id。
5. socks5.Remark = Client.Remark。
6. socks5.Status 默认 false。
```

Basic 规则：

```text
Web/API 中的 u / p 有效。
NPC 配置中的 basic_username / basic_password 可以存在，但 NPS 入库前会清空，不会覆盖服务端配置。
```

### 修改客户端 Basic 认证

```text
POST /client/basic/
```

参数：

```text
id          单个客户端 ID，和 ids 二选一
ids         批量客户端 ID，逗号分隔，例如 1,2,3
u           Basic 认证用户名，可留空
p           Basic 认证密码，可留空
timestamp
auth_key
```

行为：

```text
1. 只允许管理员或 auth_key API 调用。
2. 只修改客户端 Cnf.U / Cnf.P，不修改客户端 ID、备注、流量、速率、SOCKS5 等其它字段。
3. 批量修改会先校验全部 ID；只要有一个 ID 不存在或非法，就不会写入任何客户端。
4. 留空 u 或 p 会按空值保存，可用于统一清空 Basic 用户名或密码。
```

### 编辑客户端

```text
POST /client/edit/
```

参数：

```text
id
remark
vkey
u
p
compress
crypt
config_conn_allow
rate_limit
flow_limit
max_conn
max_tunnel
web_username
web_password
timestamp
auth_key
```

行为：

```text
1. Client.Id 不变。
2. 对应 SOCKS5 备注同步为客户端备注。
3. SOCKS5 开关状态不会被客户端编辑重置。
```

### 删除客户端

```text
POST /client/del/
```

参数：

```text
id
timestamp
auth_key
```

行为：

```text
1. 删除客户端。
2. 删除对应托管 SOCKS5。
3. 删除该客户端关联的隧道和域名解析。
4. 被删除的 Client.Id 之后可复用。
```

### 修改客户端连接状态

```text
POST /client/changestatus/
```

参数：

```text
id
status      true/false 或 1/0
timestamp
auth_key
```

该接口只控制客户端是否允许连接，不等同于 SOCKS5 开关。

## 隧道接口

### 隧道列表

```text
POST /index/gettunnel/
```

参数：

```text
type          portForward/socks5/httpProxy/secret/p2p/file
client_id     可选
offset
limit
search
timestamp
auth_key
```

说明：

```text
type=socks5 查询托管 SOCKS5 列表。
type=portForward 查询端口转发列表。
```

### 获取单个隧道

```text
POST /index/getonetunnel/
```

参数：

```text
id
type
timestamp
auth_key
```

### 新增隧道

```text
POST /index/add/
```

参数：

```text
type          portForward/httpProxy/secret/p2p/file
client_id
port
server_ip
target
remark
password
local_path
strip_pre
local_proxy   1/0
timestamp
auth_key
```

说明：

```text
type=portForward 时，一个规则同时监听 TCP 和 UDP。
type=socks5 会被拒绝，SOCKS5 只能由客户端托管生成。
新增失败不会占用 ID。
```

### 编辑隧道

```text
POST /index/edit/
```

参数：

```text
id
type
old_type
client_id
port
server_ip
target
remark
password
local_path
strip_pre
local_proxy
timestamp
auth_key
```

说明：

```text
socks5 不允许编辑。
old_type 用于准确定位原任务池。
```

### 删除隧道

```text
POST /index/del/
```

参数：

```text
id
type
timestamp
auth_key
```

说明：

```text
socks5 不允许删除。
被删除的 ID 之后可被同一 type 复用。
```

### 启动隧道

```text
POST /index/start/
```

参数：

```text
id
type
timestamp
auth_key
```

SOCKS5 开关示例：

```bash
curl -X POST "http://127.0.0.1:8080/index/start/" \
  -d "id=3" \
  -d "type=socks5" \
  -d "timestamp=<timestamp>" \
  -d "auth_key=<md5>"
```

### 停止隧道

```text
POST /index/stop/
```

参数：

```text
id
type
timestamp
auth_key
```

SOCKS5 关闭示例：

```bash
curl -X POST "http://127.0.0.1:8080/index/stop/" \
  -d "id=3" \
  -d "type=socks5" \
  -d "timestamp=<timestamp>" \
  -d "auth_key=<md5>"
```

## SOCKS5 状态字段

列表返回中重点关注：

```text
Id
Client.Id
Remark
Port
Status
RunStatus
Client.IsConnect
Flow.InletFlow
Flow.ExportFlow
```

说明：

```text
Status    表示配置开关是否开启。
RunStatus 表示当前是否真的在监听运行。
Flow      为隧道自身流量，可用于计算实时速率。
```

运行中的托管 SOCKS5 如果 30 分钟没有入口/出口流量变化，会自动关闭并持久化 `Status=false`。
