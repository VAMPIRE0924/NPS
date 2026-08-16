# NPS

本仓库基于[NPS](https://github.com/ehang-io/nps)优化，兼容旧版NPC

## 功能特性

- 每个功能面使用独立 ID 池。
- 新增对象时永远取当前池内最小可用正整数 ID。
- 删除 ID 后，下次新增可以自动复用被删除的小 ID。
- TCP/UDP 不再作为两个后台功能维护，统一为一个端口转发规则。
- 每个客户端自动绑定一个托管 SOCKS5 代理。
- SOCKS5 代理默认关闭，只允许在管理端/API 中打开或关闭。
- SOCKS5隧道无流量 30 分钟 -> 自动关闭 SOCKS 并持久化 Status=false
- NPC 上报的 Basic 认证用户名和密码无效化，Basic 只由 NPS 服务端配置控制。
- Web 写操作使用 CSRF 防护，API 使用带时间窗和 nonce 防重放的 HMAC-SHA256。
- Web 管理密码至少 12 个字符，样例占位密码不能启动管理端。

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

需要 Go 1.25 或更新版本；`go.mod` 固定使用带安全修复的 Go 1.26.6 工具链。

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

详见 [API.md](API.md)。旧版 MD5 查询参数认证已移除，升级现有 API 调用方时必须改用
`X-NPS-Timestamp`、`X-NPS-Nonce` 和 `X-NPS-Signature` 请求头。

## 首次启动与升级

复制 `conf/nps.conf` 后，先把 `web_password=CHANGE_ME_BEFORE_START` 改成至少 12 个字符的
唯一强密码。`auth_key` 默认留空（API 关闭）；需要 API 时请设置独立的长随机密钥。
生产环境应启用 TLS，并把 Web 管理端限制在受信网络。

## Docker

`Dockerfile.nps` 构建 NPS-only 镜像，支持 `linux/amd64` 和 `linux/arm64`。容器内目录：

```text
/nps/nps       NPS 服务端
/nps/web/      Web 静态资源与模板
/nps/conf/     配置和持久化数据卷
```

镜像标签：

| 标签 | 用途 |
| --- | --- |
| `main` | 通过验收并合入受保护 `main` 分支的最新稳定构建 |
| `<VERSION>` | 固定正式版本，例如 `2.0.0`，生产环境推荐使用 |
| `dev` | `dev` 分支测试构建，不建议生产使用 |

首次启动前先下载公开配置模板，并将占位管理密码替换为至少 12 位的唯一强密码：

```bash
mkdir -p ./conf
curl -fsSL \
  https://raw.githubusercontent.com/VAMPIRE0924/NPS/main/conf/nps.conf \
  -o ./conf/nps.conf
chmod 600 ./conf/nps.conf
```

示例：

```bash
docker run -d \
  --name nps \
  --restart unless-stopped \
  --network host \
  -v /宿主机/nps/conf:/nps/conf \
  vampirerune/nps:2.0.0
```

Linux 使用 `--network host` 时无需逐个映射动态隧道端口。使用 bridge 网络时，需要映射
Bridge、Web、HTTP/HTTPS 代理端口以及所有隧道端口。

也可直接使用仓库中的 `compose.yaml`：

```bash
docker compose up -d
docker compose logs -f nps
```

生产环境建议固定版本号而不是跟随 `main`。升级前备份 `conf/`，再执行
`docker compose pull && docker compose up -d`。更完整的 Docker 部署说明见
[DOCKERHUB.md](DOCKERHUB.md)。

自动化分支与发布规则：

```text
推送 dev                    验证后发布 vampirerune/nps:dev
提交 dev -> main 的 PR      执行验收测试、vet、漏洞扫描和 Docker 构建检查
在 main 提交上推送 v* 标签  自动创建 GitHub Release，上传 Linux/Windows amd64 包，
                            并发布 vampirerune/nps:<VERSION> 与 :main
```

`main` 只接受通过验收检查的 PR，日常开发在 `dev` 完成。仓库需配置：

```text
Repository variable: DOCKERHUB_USERNAME
Repository variable: DOCKERHUB_REPOSITORY（可选，默认 nps）
Repository secret:   DOCKERHUB_TOKEN
```

`DOCKERHUB_TOKEN` 必须使用具备目标仓库读写权限的 Docker Hub Personal Access Token，
不要使用 Docker Hub 登录密码。
