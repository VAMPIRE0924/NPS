# NPS 重构版 API 说明

本文是管理平台适配本仓库 NPS 服务端的当前 HTTP 合约，源码与测试以本仓库为准。

版本边界：

- `v2.0.0` 是当前正式版本基线；`Unreleased` 记录的下一次 `main` 候选继续使用同一套
  HMAC-SHA256、复合任务 ID 和 `POST /index/socksstatus/` 合约。
- 签名按实际发送的原始正文字节计算；空正文伪签名、过期时间戳、nonce 重放和错误签名均拒绝。
- 管理 API 已完成 Client、Host、五类可手工创建隧道，以及随 Client 自动创建的托管 SOCKS 的
  生产生命周期回归。

## 1. 基础约定

### 1.1 服务地址

接口路径受 `nps.conf` 的 `web_base_url` 影响：

```text
web_base_url=       -> /client/list/
web_base_url=/nps   -> /nps/client/list/
```

外部系统应配置一个统一 `baseURL`，不要在业务代码中重复拼接前缀。

### 1.2 请求格式

除 `/auth/gettime/` 外，管理 API 推荐统一使用：

```http
POST
Content-Type: application/x-www-form-urlencoded
```

签名覆盖的是编码后的原始请求体字节。生成签名后不得重新排序、重新编码或修改请求体。

### 1.3 响应格式

历史接口存在三种响应形状：

```json
{"status":1,"msg":"success"}
{"rows":[],"total":0}
{"code":1,"data":{}}
```

失败示例：

```json
{"status":0,"msg":"business error"}
{"code":0,"msg":"not found"}
```

适配层必须同时判断 HTTP 状态码和 JSON 中的 `status` 或 `code`。多数业务错误仍返回
HTTP 200；鉴权失败返回 HTTP 401，方法错误返回 HTTP 405。
未建立 Web 会话且未携带完整 HMAC 请求头的受保护 POST 也返回 HTTP 401，
不跳转登录页。

## 2. API 鉴权

本章接口全部是管理员管理 API。客户端账号和 VerifyKey Web 登录不会获得 API 身份；客户端页面
使用受 CSRF 保护的 Web 会话，并由服务端强制限定为本 Client。不能把浏览器客户会话当作平台 API 凭据。

### 2.1 配置

在 `nps.conf` 中设置独立的长随机密钥：

```ini
auth_key=<仅保存在生产密钥系统中的随机密钥>
```

`auth_key` 非空时至少 32 个字符，建议使用密码学安全随机源生成 32 字节以上随机值。它不得与
`web_password`、`public_vkey` 或任何 Client `VerifyKey` 复用。外部调用必须
通过 TLS；不得把密钥、完整签名请求或包含密码的响应写入日志。

运行后 `auth_key` 在 `nps.conf` 中以 `npsenc:v1:` 密文落盘，解密值只保留在进程内；管理 API
对 Client VerifyKey、Web/Basic 等字段的既有查询结果不变。迁移必须连同 `conf/credential.key` 整体备份 `conf/`。

### 2.2 请求头

每次请求必须生成新的时间戳和 nonce：

```text
X-NPS-Timestamp: Unix 秒级时间戳
X-NPS-Nonce: 16-128 位，仅允许 A-Z a-z 0-9 _ -
X-NPS-Signature: HMAC-SHA256 十六进制小写结果
```

### 2.3 签名规范

规范字符串：

```text
METHOD + "\n" + PATH_WITH_RAW_QUERY + "\n" + TIMESTAMP + "\n" + NONCE + "\n" + SHA256_HEX(BODY)
```

说明：

- `METHOD` 使用大写，例如 `POST`。
- `PATH_WITH_RAW_QUERY` 是转义后的路径；存在查询参数时追加 `?` 和原始查询串。
- `BODY` 是实际发送的完整原始字节；空请求体也要计算 SHA-256。
- 以 `auth_key` 原始文本为 HMAC-SHA256 密钥，输出小写十六进制。
- 服务端允许正负 30 秒时钟偏差。
- nonce 在当前 NPS 进程内约 60 秒不可重复。
- 最大请求体为 1 MiB。
- 旧 `MD5(auth_key + timestamp)` 查询参数鉴权已移除。

服务器时间可匿名读取：

```text
GET /auth/gettime/
```

```json
{"time":1800000000}
```

完整调用代码见[签名与调用示例](API_SIGNING_EXAMPLES.md)。

## 3. ID 与模式

新增对象由服务端分配对应池内最小可用正整数。调用方不得提交或自行推算
`max(id) + 1`。

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

任务唯一键是 `type + ":" + id`。例如 `portForward:1` 和 `socks5:1` 是两个不同任务。
调用任务详情、编辑、删除、启动和停止接口时必须携带真实 `type`。

新平台允许的任务类型：

```text
portForward  socks5  httpProxy  secret  p2p  file
```

`portForward` 同时承载 TCP 和 UDP。新平台不得建立独立 UDP 类型或 ID 池。

## 4. 客户端 API

### 4.1 接口表

| 操作 | 方法与路径 | 参数 |
| --- | --- | --- |
| 列表 | `POST /client/list/` | `offset limit search sort order` |
| 详情 | `POST /client/getclient/` | `id` |
| 新增 | `POST /client/add/` | 见下方字段 |
| 编辑 | `POST /client/edit/` | 新增字段加 `id` |
| 修改连接权限 | `POST /client/changestatus/` | `id status` |
| 修改 Basic | `POST /client/basic/` | `id` 或 `ids`，以及 `u p` |
| 删除 | `POST /client/del/` | `id` |

新增/编辑字段：

```text
remark
vkey                 留空时由服务端生成
u
p
compress             1/0
crypt                1/0
config_conn_allow    1/0
rate_limit           KB/s，0 表示不限制
flow_limit           MB，0 表示不限制
max_conn              0 表示不限制
max_tunnel            0 表示不限制
web_username
web_password
```

新增可见 Client 时会自动创建一个默认关闭的托管 SOCKS：

```text
socks5.Id        = Client.Id
socks5.Client.Id = Client.Id
socks5.Port      = 10000 + Client.Id
socks5.Remark    = Client.Remark
socks5.Status    = false
```

删除 Client 会同时删除对应托管 SOCKS、关联隧道和域名。Client 编辑是完整表单更新，不是
PATCH；调用前应读取详情并提交完整字段，避免遗漏字段被清空。

`/client/basic/` 的 `ids` 使用英文逗号分隔，例如 `1,2,3`。批量更新会先校验全部 ID，任一
无效时不写入任何客户端。

新增接口成功响应不返回新 ID。适配层应使用调用前生成的唯一 `vkey` 查回 Client，不能按最大
ID 推断结果。

## 5. 隧道 API

### 5.1 接口表

| 操作 | 方法与路径 | 参数 |
| --- | --- | --- |
| 列表 | `POST /index/gettunnel/` | `type client_id offset limit search` |
| 详情 | `POST /index/getonetunnel/` | `id type` |
| 新增 | `POST /index/add/` | `type client_id` 及模式字段 |
| 编辑 | `POST /index/edit/` | `id type old_type` 及完整模式字段 |
| 删除 | `POST /index/del/` | `id type` |
| 启动 | `POST /index/start/` | `id type` |
| 停止 | `POST /index/stop/` | `id type` |

模式字段按需包含：

```text
port
server_ip
target
remark
password
local_path
strip_pre
local_proxy    1/0
```

重要行为：

- `type=portForward` 的一个规则在相同端口同时监听 TCP 和 UDP。
- `type=socks5` 只允许列表、详情、状态查询、启动和停止；新增、编辑、删除会被拒绝。
- 编辑接口是完整表单更新，不是 PATCH；切换任务类型时用 `old_type` 定位旧任务。
- 删除后 ID 可在同一类型池内复用。
- 创建失败不占用 ID。

## 6. 托管 SOCKS 状态 API

### 6.1 请求

```text
POST /index/socksstatus/
```

请求体二选一；两者数值都等于 Client.Id：

```text
id=3
client_id=3
```

成功响应示例（正在运行，但当前无流量活动）：

```json
{
  "code": 1,
  "data": {
    "id": 3,
    "client_id": 3,
    "enabled": true,
    "running": true,
    "active": false,
    "countdown": true,
    "last_active_at": 1800000000,
    "idle_seconds": 600,
    "remaining_seconds": 1200,
    "auto_close_at": 1800001200,
    "auto_close_timeout_seconds": 1800,
    "inlet_flow": 1024,
    "export_flow": 2048
  }
}
```

未找到：

```json
{"code":0,"msg":"managed socks5 tunnel not found"}
```

### 6.2 字段语义

| 字段 | 含义 |
| --- | --- |
| `enabled` | 持久化配置开关 `Tunnel.Status` |
| `running` | 服务是否存在于 NPS 实际运行表；判断是否正在监听应优先看它 |
| `active` | 入口或出口累计流量在最近一次采样周期内发生变化 |
| `countdown` | 已运行但当前无流量活动，正在执行空闲关闭倒计时 |
| `last_active_at` | 计时器最近一次观察到流量变化的 Unix 秒；停止时为 0 |
| `idle_seconds` | 自最近流量变化起的空闲秒数；停止时为 0 |
| `remaining_seconds` | 预计距离自动关闭的剩余秒数；停止时为 0 |
| `auto_close_at` | 预计自动关闭的 Unix 秒；停止时为 0 |
| `auto_close_timeout_seconds` | 当前固定空闲阈值，现为 1800 秒 |
| `inlet_flow` / `export_flow` | SOCKS 隧道累计入口/出口字节数 |

`active` 表示流量活动，不代表 Client 在线，也不等同于 `running`。状态采样和自动关闭检查每分钟
执行一次，因此 `remaining_seconds` 和 `auto_close_at` 是估算值，实际停止最多可能晚约一个采样
周期。查询接口只读取状态，不会刷新或延长空闲计时器。

适配建议：

```text
running=false                 -> 显示“已停止”，不展示倒计时
running=true, active=true     -> 显示“流量活跃”
running=true, active=false    -> 显示“空闲，将在 remaining_seconds 秒后关闭”
```

## 7. 域名 API

| 操作 | 方法与路径 | 参数 |
| --- | --- | --- |
| 列表 | `POST /index/hostlist/` | `client_id offset limit search` |
| 详情 | `POST /index/gethost/` | `id` |
| 新增 | `POST /index/addhost/` | 见下方字段 |
| 编辑 | `POST /index/edithost/` | 新增字段加 `id` |
| 删除 | `POST /index/delhost/` | `id` |

新增/编辑字段：

```text
client_id host scheme location target remark header hostchange
key_file_path cert_file_path local_proxy
```

Host.Id 由独立池分配最小可用正整数。编辑同样按完整表单处理。

## 8. 数据与安全注意事项

旧模型直接序列化 Go 对象，详情和列表可能包含 `VerifyKey`、Basic 密码、Web 密码、隧道密码
或嵌套配置。管理平台后端必须：

- 只把业务需要的字段映射到自有 DTO，不得原样透传到前端。
- 对密钥和密码字段做遮罩，并禁止写入应用日志、追踪系统和错误上报。
- 在受信网络内调用 NPS，并强制验证 TLS 证书。
- 客户端列表可省略 `sort`，在适配层使用字段白名单排序。
- 任务主键保存为 `(type, id)`；只有托管 SOCKS 可按 Client.Id 直接定位。

## 9. 推荐适配层模型

管理平台不要把 NPS 原始 JSON 直接作为领域模型。建议至少保存：

```text
NpsClientRef     { npsInstanceId, clientId }
NpsTunnelRef     { npsInstanceId, type, tunnelId }
NpsHostRef       { npsInstanceId, hostId }
SocksRuntime     { running, active, remainingSeconds, observedAt }
```

每次状态轮询都记录本地 `observedAt`，展示倒计时时用服务器返回的 `auto_close_at`，并定期重新
查询，不要只依赖浏览器本地递减。
