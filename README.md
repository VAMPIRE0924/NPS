# NPS

本仓库提供 NPS 服务端源码、Web 管理端源码、必要配置样例和 API 文档。现有原版或第三方 NPC 客户端保持兼容，不需要重新编译或替换。

## 功能特性

- 每个功能面使用独立 ID 池。
- 新增对象时永远取当前池内最小可用正整数 ID。
- 删除 ID 后，下次新增可以自动复用被删除的小 ID。
- TCP/UDP 不再作为两个后台功能维护，统一为一个端口转发规则。
- 每个客户端自动绑定一个托管 SOCKS5 代理。
- SOCKS5 代理默认关闭，只允许在管理端/API 中打开或关闭。
- NPC 上报的 Basic 认证用户名和密码无效化，Basic 只由 NPS 服务端配置控制。

## ID 规则

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

任务内部唯一键为：

```text
mode:id
```

例如：

```text
portForward:1
socks5:1
httpProxy:1
```

这意味着不同功能可以同时拥有自己的 `ID=1`，互不冲突。

## SOCKS5 托管规则

新增客户端后，系统自动创建对应的 SOCKS5 代理：

```text
socks5.Id        = Client.Id
socks5.Client.Id = Client.Id
socks5.Port      = 10000 + Client.Id
socks5.Remark    = Client.Remark
```

行为：

```text
新增客户端      -> 自动创建关闭状态的 SOCKS5
修改客户端备注  -> 同步 SOCKS5 备注
删除客户端      -> 删除对应 SOCKS5
SOCKS5 页面     -> 只能查看和开关，不能新增/编辑/删除
无流量 30 分钟  -> 自动关闭 SOCKS5 并持久化 Status=false
```

## 端口转发

端口转发统一使用：

```text
portForward
```

一个 `portForward` 规则会同时监听同一个端口的 TCP 和 UDP。

旧 NPC 配置中如果上报 `mode=tcp` 或 `mode=udp`，NPS 会在服务端归一为 `portForward`，但实际链路协议仍然保持 NPC 原有的 `CONN_TCP` / `CONN_UDP`，所以现有 NPC 不需要改。

## Basic 认证

Basic 认证用户名和密码只能由 NPS 服务端配置：

```text
Web/API 中的 u / p                       -> 有效
NPC 配置中的 basic_username/basic_password -> NPS 入库前清空，不采信
```

Web 客户端列表支持单独或批量修改 Basic 认证用户名和密码；也可以调用 `/client/basic/` 接口完成同样操作。

## 构建

Windows amd64:

```powershell
go build -trimpath -ldflags='-s -w' -o dist/windows_amd64/nps.exe ./cmd/nps
```

Linux amd64:

```powershell
$env:GOOS='linux'
$env:GOARCH='amd64'
go build -trimpath -ldflags='-s -w' -o dist/linux_amd64/nps ./cmd/nps
Remove-Item Env:\GOOS
Remove-Item Env:\GOARCH
```

部署时需要同时带上匹配源码版本的 `web/` 目录：

```text
nps
conf/
web/
  static/
  views/
```

只替换二进制时，请确认服务器上的 `web/` 目录已经同步到本仓库版本。

## API 文档

详见 [API.md](API.md)。
