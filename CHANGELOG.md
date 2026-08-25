# 变更记录

本文件记录本仓库相对已发布版本的用户可见变化。正式版本由受保护 `main` 上的 `v*` 标签产生。

## Unreleased

暂无。

## v2.0.2 - 2026-08-25

### NPC 与权限

- 修复正式 Client 关闭 NPC 配置上报后，旧 NPC 使用配置文件启动时被断开、无法进入
  主连接的问题。
- `ConfigConnAllow=false` 现只允许 NPC 上线，客户端上报的 Client/Host/Tunnel/状态不生效；
  服务端创建的规则仍可通过该 NPC 正常工作。
- 保持旧 NPC 线协议、配置文件和 `public_vkey` 公共配置模式兼容，不需要替换 NPC。

### Client 身份与 Web

- 移除每 Client Web 用户名/密码的新增、编辑、列表、注册和登录路径；客户端 Web
  只使用固定用户名 `user` + Client VerifyKey。历史字段在加载时清理。
- VerifyKey 新增/编辑不再被 HTML 转义，避免服务端身份与 NPC 实际密钥不一致。
- 编辑时留空会重新生成 VerifyKey，非空重复值会被拒绝；历史空 VerifyKey 在加载时自动轮换为
  新的 16 位密码学随机密钥，并以 AES-256-GCM 密文回写。

### 验证

- OpenWrt 上未修改的原 NPC 已完成 `ConfigConnAllow=false` 真实上线与服务端管理规则验证。
- Go 1.26.6 的 NPS 定向测试、关键包 race、vet、可达漏洞扫描和 amd64/arm64 镜像构建通过。

## v2.0.1 - 2026-08-17

本节记录 `v2.0.1` 相对 `v2.0.0` 的变更：

### 安全

- 管理 API 保持管理员专用；普通客户 Web 会话强制绑定 Session 中的 Client ID。
- Client、Host 和 Tunnel 的列表、详情与写操作增加对象归属校验，跨客户访问统一返回 HTTP 403。
- Web 写操作继续要求 CSRF；无认证、错误 HMAC、过期时间戳和 nonce 重放返回 HTTP 401。
- `nps.conf`、`clients.json`、`tasks.json`、`hosts.json` 的凭据字段改为 AES-256-GCM 非明文落盘。
- 增加代理鉴权头清理、共享缓存隔离、Host 泛域名边界、stored XSS、异常帧长度、握手超时、
  SOCKS/P2P 并发和拒绝服务防护。
- 移除仓库中的示例 TLS 私钥；启用 TLS 终止时必须使用部署者自己的证书和私钥。

### Web 与兼容性

- 修复任务按钮携带错误模式或 ID 导致暂停、启动、编辑、删除失败的问题。
- 修复旧浏览器缓存继续使用过期静态资源的问题，并恢复 `file` 隧道的新增和编辑表单。
- 权限拒绝现在返回明确 HTTP 403 JSON，不再出现空 HTTP 200。
- 普通客户仍可管理自己的 Host、`secret`、`p2p` 并启停自己的托管 SOCKS；独占 NPS 端口的
  `portForward`、`httpProxy`、`file` 由管理员管理。
- NPC 线协议、VerifyKey Web 登录、客户端/客户端内网目标、LocalProxy 和 API 查询字段保持兼容。

### 验证

- Go 1.26.6 的 NPS 定向测试、关键包 race、vet 和可达漏洞扫描通过。
- 生产管理员与客户 Web 全页面回归通过；跨客户越权、CSRF、Session、Cookie 和 HMAC 边界通过。
- 生产 HMAC API 对 Client、Host、五类可手工创建隧道，以及随 Client 自动创建的托管 SOCKS，
  完成了各自允许的新增、详情、编辑、启停、状态和删除生命周期验证；所有临时对象已清理。
- Client 1 的 TCP/UDP `portForward` 经 NPC 到客户端内网的真实数据链路通过。

### 升级提醒

- 升级前必须备份完整 `conf/`。首次启动会生成 `conf/credential.key` 并迁移凭据为密文。
- 回滚到旧版本时必须恢复升级前的完整 `conf/`；旧版本不能直接使用迁移后的密文配置。
- NPS 二进制与完整 `web/` 必须作为同一版本单元升级。

完整步骤见 [UPGRADING.md](UPGRADING.md)。
