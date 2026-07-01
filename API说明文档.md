# NPS 重构版 API 说明文档

本文档面向后期设备管理平台。当前后端源码以 `workspace/nps-dev` 为准。

本次二进制体积优化只调整 release 构建参数和少量服务端代码质量点，不改变 API 路径、请求参数或返回字段。

部署边界：本重构只需要替换 NPS 服务端。现有原版或第三方 NPC 二进制与配置继续兼容，不需要重新编译或替换。

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

通用成功/失败响应：

```json
{"status":1,"msg":"success message"}
{"status":0,"msg":"error message"}
```

列表响应：

```json
{"rows":[],"total":0}
```

详情响应：

```json
{"code":1,"data":{}}
{"code":0}
```

## 鉴权

保留 NPS 原有 `auth_key` 方式。请求附带：

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
  -d "auth_key=<md5结果>"
```

## ID 规则

服务端新增对象时自动分配当前池内最小可用正整数。不要在管理平台里自己计算 `max(id)+1`。

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

示例：

```text
portForward:1
socks5:1
httpProxy:1
```

`portForward:1` 和 `socks5:1` 可以同时存在。端口转发只使用 `type=portForward`，不要再拆成 TCP/UDP 两套任务。

## 客户端 API

### 客户端列表

```text
POST /client/list/
```

参数：

```text
offset
limit
search       可按 ID、vkey、备注搜索
sort
order        asc/desc
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
u                   basic 用户名
p                   basic 密码
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

说明：

```text
u / p 是服务端管理侧配置的 Basic 认证用户名和密码。
NPC 客户端配置里的 basic_username / basic_password 可以继续存在，但 NPS 入库前会清空，不会覆盖服务端配置。
```

行为：

```text
1. Client.Id 自动取最小可用正整数。
2. 自动创建 managed SOCKS。
3. SOCKS ID = Client.Id。
4. SOCKS 端口 = 10000 + Client.Id。
5. SOCKS 备注 = Client.Remark。
6. SOCKS 默认关闭。
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
2. managed SOCKS 备注同步为客户端备注。
3. managed SOCKS 开关状态不会被客户端编辑重置。
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
2. 删除对应 managed SOCKS。
3. 删除该客户端关联的隧道和域名解析。
4. 被删除的 Client.Id 后续可被复用。
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

说明：该接口只控制客户端是否允许连接，不等同于 SOCKS 开关。

## SOCKS API

SOCKS 是客户端绑定的托管隧道，不允许手动新增、编辑、删除。

绑定规则：

```text
socks5.Id        = Client.Id
socks5.Client.Id = Client.Id
socks5.Port      = 10000 + Client.Id
socks5.Remark    = Client.Remark
```

### 查询 SOCKS 列表

```text
POST /index/gettunnel/
```

参数：

```text
type=socks5
offset
limit
search
timestamp
auth_key
```

关键字段：

```text
Id                SOCKS ID，即 Client.Id
Port              10000 + Client.Id
Status            持久化开关
RunStatus         当前进程内是否正在监听
Client.Id         绑定客户端 ID
Remark            与客户端备注同步
Flow.InletFlow    累计入口流量，byte
Flow.ExportFlow   累计出口流量，byte
```

API 返回累计流量，不直接返回速率。管理平台显示速率时，每 3-5 秒请求一次列表，用两次 `Flow.InletFlow` / `Flow.ExportFlow` 差值除以时间差计算。

### 打开 SOCKS

```text
POST /index/start/
```

参数：

```text
id        Client.Id，也是 socks5.Id
type      socks5
timestamp
auth_key
```

成功：

```json
{"status":1,"msg":"start success"}
```

### 关闭 SOCKS

```text
POST /index/stop/
```

参数：

```text
id
type=socks5
timestamp
auth_key
```

成功：

```json
{"status":1,"msg":"stop success"}
```

### 空闲自动关闭

打开后的 managed SOCKS 如果 30 分钟没有入口或出口流量变化，服务端会自动关闭并持久化：

```text
Status=false
```

再次调用 `/index/start/` 可以重新打开。

## 通用隧道 API

支持类型：

```text
portForward
httpProxy
secret
p2p
file
```

`type=socks5` 不允许走新增、编辑、删除；SOCKS 只能跟随客户端自动管理。

### 隧道列表

```text
POST /index/gettunnel/
```

参数：

```text
type        可选。为空且 client_id 也为空时返回全部隧道。
client_id   可选
offset
limit
search
timestamp
auth_key
```

常用字段：

```text
Id
Mode
Port
Status
RunStatus
Client.Id
Remark
Target.TargetStr
Flow.InletFlow
Flow.ExportFlow
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
password      secret/p2p 密钥
local_path    file 模式本地路径
strip_pre     file 模式路径前缀
local_proxy   1/0
timestamp
auth_key
```

说明：

```text
type=portForward 时，一个规则同时监听 TCP 和 UDP。
type=socks5 会被拒绝。
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
old_type      可选，模式变化时传旧模式
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
type=socks5 或 old_type=socks5 会被拒绝。
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
type=socks5 会被拒绝。
删除后该 type 池内 ID 可被复用。
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

成功后持久化 `Status=true`。

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

成功后持久化 `Status=false`。

## 域名解析 API

### 域名列表

```text
POST /index/hostlist/
```

参数：

```text
client_id
offset
limit
search
timestamp
auth_key
```

### 获取单条域名解析

```text
POST /index/gethost/
```

参数：

```text
id
timestamp
auth_key
```

### 新增域名解析

```text
POST /index/addhost/
```

参数：

```text
client_id
host
scheme        all/http/https
location      URL 路由，默认 /
target
remark
header
hostchange
key_file_path
cert_file_path
local_proxy
timestamp
auth_key
```

### 编辑域名解析

```text
POST /index/edithost/
```

参数同新增，额外需要：

```text
id
```

### 删除域名解析

```text
POST /index/delhost/
```

参数：

```text
id
timestamp
auth_key
```

## 管理平台建议

1. 新增客户端后，通过 `/client/list/` 按唯一 `vkey` 或备注查回最终 `Client.Id`。
2. SOCKS 端口固定按 `10000 + Client.Id` 计算，不手动创建 SOCKS。
3. SOCKS 开关调用 `/index/start/` 和 `/index/stop/`，必须传 `type=socks5`。
4. 端口转发统一使用 `type=portForward`，一条规则同时处理 TCP 和 UDP。
5. 不要在新平台中重新拆出独立 TCP/UDP 隧道池。
6. 显示实时速率时，用隧道自身 `Flow.InletFlow` 和 `Flow.ExportFlow` 做差值，不依赖客户端总流量。
7. 展示开关状态优先看 `Status`；展示当前是否真的在监听时看 `RunStatus`。
