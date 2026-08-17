# NPS 服务端增强版

[![NPS acceptance](https://github.com/VAMPIRE0924/NPS/actions/workflows/acceptance.yml/badge.svg?branch=main)](https://github.com/VAMPIRE0924/NPS/actions/workflows/acceptance.yml)
[![Docker Pulls](https://img.shields.io/docker/pulls/vampirerune/nps)](https://hub.docker.com/r/vampirerune/nps)
[![License](https://img.shields.io/github/license/VAMPIRE0924/NPS)](LICENSE)

本仓库基于 [ehang-io/nps](https://github.com/ehang-io/nps) 维护 NPS 服务端增强版，重点改进
任务模型、Web/API 权限边界、凭据落盘和自动发布。它不是上游官方仓库。

现有原版或第三方 NPC 二进制及配置无需替换。发布物只包含 NPS 服务端；NPC 线协议、客户端本机
与客户端内网目标访问、`LocalProxy` 和旧 `public_vkey` 临时配置模式保持兼容。

## 核心能力

- Client、Host 和每种隧道模式使用独立 ID 池，新增对象复用本池最小可用正整数。
- 任务使用 `mode:id` 复合键，不同模式可同时拥有相同数字 ID。
- `tcp`、`udp` 在服务端归一为一个 `portForward` 规则，同端口同时监听 TCP 和 UDP。
- 每个可见 Client 自动绑定同 ID 的托管 SOCKS5，默认关闭，连续 30 分钟无流量后自动停止。
- 管理 API 使用覆盖原始正文的 HMAC-SHA256、30 秒时间窗和 nonce 防重放；旧 MD5 API 已移除。
- Web 写操作使用 POST 与 CSRF；登录成功轮换 Session ID，Cookie 使用安全属性。
- 普通客户可以用本 Client 的 VerifyKey 登录 Web，但只能查看自己的对象，不获得管理 API 权限。
- `nps.conf` 与持久化 JSON 中的凭据使用 AES-256-GCM 非明文落盘。
- 现有 Web/API 查询仍读取进程内明文值，不改变 NPC、Web 登录或管理平台调用语义。

详细变更见 [CHANGELOG.md](CHANGELOG.md)，安全边界见 [SECURITY.md](SECURITY.md)。

## 权限边界

| 功能 | 管理员 Web / HMAC API | 普通客户 Web |
| --- | --- | --- |
| 查看 Client、Host、Tunnel | 全部 | 仅本 Client |
| Host 路由 | 管理 | 管理自己的路由，不可设置服务端证书私钥路径 |
| `secret` / `p2p` | 管理 | 管理自己的规则 |
| 托管 SOCKS5 | 查询、启停 | 仅查询、启停自己的 SOCKS5 |
| `portForward` / `httpProxy` / `file` | 管理 | 只读，不可新增、编辑、删除或启停 |
| Client 状态与配置上报权限 | 管理 | 不可修改 |
| 管理 API | 可用 | 不可用 |

这些限制只约束控制面，不限制 NPS/NPC 原有数据面访问客户端及客户端内网的能力。

## Docker 快速开始

镜像：[`vampirerune/nps`](https://hub.docker.com/r/vampirerune/nps)，支持
`linux/amd64`、`linux/arm64`。

| 标签 | 用途 |
| --- | --- |
| `main` | 受保护 `main` 分支通过验收后的稳定通道 |
| `<VERSION>` | 固定正式版本，生产推荐使用 |
| `dev` | 开发验收通道，不建议生产长期使用 |

首次部署先准备配置。示例密码故意不可用，必须改成至少 12 个字符的唯一强密码：

```bash
mkdir -p ./conf
curl -fsSL \
  https://raw.githubusercontent.com/VAMPIRE0924/NPS/main/conf/nps.conf \
  -o ./conf/nps.conf
chmod 600 ./conf/nps.conf
```

不使用 API 或公共配置模式时，保持 `auth_key=`、`public_vkey=` 为空。不要提交真实配置、JSON、
证书私钥或 `credential.key`。

Linux 推荐 host 网络，因为 NPS 会动态监听隧道端口：

```yaml
services:
  nps:
    image: vampirerune/nps:main
    container_name: nps
    restart: unless-stopped
    network_mode: host
    volumes:
      - ./conf:/nps/conf
```

```bash
docker compose up -d
docker compose logs -f nps
```

bridge 网络必须显式映射 Web、NPC Bridge、HTTP/HTTPS 和每个隧道端口；端口转发规则需要同时
映射 TCP 与 UDP。完整说明见 [DOCKERHUB.md](DOCKERHUB.md)。

## 首次启动、备份与迁移

第一次成功启动会生成权限为 `0600` 的 `conf/credential.key`，并把以下凭据迁移为
`npsenc:v1:` 密文：

- `nps.conf`：`web_password`、`auth_key`、`public_vkey`；
- `clients.json`：VerifyKey、Web 密码、Basic 密码；
- `tasks.json`、`hosts.json`：隧道、Host 和多账号凭据。

密文不会在停止服务后恢复为明文。备份、迁移和恢复必须整体复制同一份 `conf/`；缺少或错配
`credential.key` 时 NPS 会拒绝启动，防止静默损坏数据。

```bash
tar -C /path/to/nps -czf nps-conf-backup.tar.gz conf
```

升级前请先阅读 [UPGRADING.md](UPGRADING.md)。旧版本不能正确读取迁移后的密文，回滚必须同时
恢复升级前的完整 `conf/` 备份。

## 非 Docker 部署

从 [Releases](https://github.com/VAMPIRE0924/NPS/releases) 下载对应平台包。NPS 不嵌入 Web
资源，部署必须使用同一版本的完整单元：

```text
nps（Windows 为 nps.exe）
conf/
web/static/
web/views/
```

本项目不要求重新构建、替换或改写 NPC。

## API

管理 API 只接受管理员 HMAC 身份。调用方必须同时判断 HTTP 状态码和历史 JSON 中的
`status` / `code` 字段：

- [API 合约](API.md)
- [HMAC 签名示例](API_SIGNING_EXAMPLES.md)

不要把 API 原始响应直接透传给前端；部分历史对象包含 VerifyKey 或密码字段，应先映射到自有 DTO
并遮罩敏感值。

## 源码构建

项目要求 Go 1.25 及以上，并固定使用 Go 1.26.6 工具链。只构建 NPS 服务端：

```bash
go test ./bridge ./client ./cmd/nps ./lib/... ./server/... ./web/...
CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags='-s -w' -o dist/nps ./cmd/nps
```

发布包还必须复制匹配版本的完整 `web/static` 与 `web/views`。正式 Release 和多架构镜像由
GitHub Actions 从受保护 `main` 及其 `v*` 标签生成。

## 上游与许可证

本项目继续遵循仓库中的 [GPL-3.0 License](LICENSE)。原项目文档和 NPC 使用方式请参考
[ehang-io/nps](https://github.com/ehang-io/nps)。
