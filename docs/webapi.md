获取客户端列表

```
POST /client/list/
```


| 参数 | 含义 |
| --- | --- |
| search | 搜索 |
| order | 排序asc 正序 desc倒序 |
| offset | 分页(第几页) |
| limit | 条数(分页显示的条数) |

***
获取单个客户端

```
POST /client/getclient/
```


| 参数 | 含义 |
| --- | --- |
| id | 客户端id |

***
添加客户端

```
POST /client/add/
```

| 参数 | 含义 |
| --- | --- |
| remark | 备注 |
| u | basic权限认证用户名 |
| p | basic权限认证密码 |
| limit | 条数(分页显示的条数) |
| vkey | 客户端验证密钥 |
| config\_conn\_allow | 是否允许客户端以配置文件模式连接 1允许 0不允许 |
| compress | 压缩1允许 0不允许 |
| crypt | 是否加密（1或者0）1允许 0不允许 |
| rate\_limit | 带宽限制 单位KB/S 空则为不限制 |
| flow\_limit | 流量限制 单位M 空则为不限制 |
| max\_conn | 客户端最大连接数量 空则为不限制 |
| max\_tunnel | 客户端最大隧道数量 空则为不限制 |

`u` / `p` are server-side Basic auth settings. NPC config values
`basic_username` / `basic_password` may still be sent by original or
third-party NPC binaries, but NPS clears them at the server receive boundary
before saving the client record.

***
修改客户端

```
POST /client/edit/
```

| 参数 | 含义 |
| --- | --- |
| remark | 备注 |
| u | basic权限认证用户名 |
| p | basic权限认证密码 |
| limit | 条数(分页显示的条数) |
| vkey | 客户端验证密钥 |
| config\_conn\_allow | 是否允许客户端以配置文件模式连接 1允许 0不允许 |
| compress | 压缩1允许 0不允许 |
| crypt | 是否加密（1或者0）1允许 0不允许 |
| rate\_limit | 带宽限制 单位KB/S 空则为不限制 |
| flow\_limit | 流量限制 单位M 空则为不限制 |
| max\_conn | 客户端最大连接数量 空则为不限制 |
| max\_tunnel | 客户端最大隧道数量 空则为不限制 |
| id | 要修改的客户端id |

***
删除客户端

```
POST /client/del/
```

| 参数 | 含义 |
| --- | --- |
| id | 要删除的客户端id |

***
获取域名解析列表

```
POST /index/hostlist/
```

| 参数 | 含义 |
| --- | --- |
| search | 搜索(可以搜域名/备注什么的) |
| offset | 分页(第几页) |
| limit | 条数(分页显示的条数) |

***
添加域名解析

```
POST /index/addhost/
```


| 参数 | 含义 |
| --- | --- |
| remark | 备注 |
| host | 域名 |
| scheme | 协议类型(三种 all http https) |
| location | url路由 空则为不限制 |
| client\_id | 客户端id |
| target | 内网目标(ip:端口) |
| header | request header 请求头 |
| hostchange | request host 请求主机 |

***
修改域名解析

```
POST /index/edithost/
```

| 参数 | 含义 |
| --- | --- |
| remark | 备注 |
| host | 域名 |
| scheme | 协议类型(三种 all http https) |
| location | url路由 空则为不限制 |
| client\_id | 客户端id |
| target | 内网目标(ip:端口) |
| header | request header 请求头 |
| hostchange | request host 请求主机 |
| id | 需要修改的域名解析id |

***
删除域名解析

```
POST /index/delhost/
```

| 参数 | 含义 |
| --- | --- |
| id | 需要删除的域名解析id |

***
获取单条隧道信息

```
POST /index/getonetunnel/
```

| 参数 | 含义 |
| --- | --- |
| id | 隧道的id |

***
获取隧道列表

```
POST /index/gettunnel/
```

| 参数 | 含义 |
| --- | --- |
| client\_id | 穿透隧道的客户端id |
| type | 类型 portForward/httpProxy/socks5/secret/p2p/file |
| search | 搜索 |
| offset | 分页(第几页) |
| limit | 条数(分页显示的条数) |

***
添加隧道

```
POST /index/add/
```

| 参数 | 含义 |
| --- | --- |
| type | 类型 portForward/httpProxy/secret/p2p/file，socks5 不允许手动新增 |
| remark | 备注 |
| port | 服务端端口 |
| target | 目标(ip:端口) |
| client\_id | 客户端id |

***
修改隧道

```
POST /index/edit/
```

| 参数 | 含义 |
| --- | --- |
| type | 类型 portForward/httpProxy/secret/p2p/file，socks5 不允许手动编辑 |
| remark | 备注 |
| port | 服务端端口 |
| target | 目标(ip:端口) |
| client\_id | 客户端id |
| id | 隧道id |

***
删除隧道

```
POST /index/del/
```

| 参数 | 含义 |
| --- | --- |
| id | 隧道id |
| type | 隧道类型，建议必传；socks5 不允许删除 |

***
隧道停止工作

```
POST /index/stop/
```

| 参数 | 含义 |
| --- | --- |
| id | 隧道id |
| type | 隧道类型，建议必传；socks5 允许关闭 |

***
隧道开始工作

```
POST /index/start/
```

| 参数 | 含义 |
| --- | --- |
| id | 隧道id |
| type | 隧道类型，建议必传；socks5 允许打开 |

ID pool refactor note

Task/tunnel IDs are independent per user-facing tunnel type. TCP and UDP are
not separate task pools; use one `portForward` task and NPS will listen on both
TCP and UDP for the configured server port.

For task APIs, new callers should pass `type` together with `id`:

```text
POST /index/getonetunnel/  id=1&type=portForward
POST /index/edit/          id=1&type=portForward
POST /index/del/           id=1&type=portForward
POST /index/stop/          id=1&type=portForward
POST /index/start/         id=1&type=portForward
```

Existing NPC clients do not need to be rebuilt. Server-side config input with
`mode=tcp` or `mode=udp` is stored as `portForward`; the wire protocol still
uses `CONN_TCP` and `CONN_UDP`.

SOCKS (`type=socks5`) is managed by the client record:

```text
socks5.Id        = Client.Id
socks5.Client.Id = Client.Id
socks5.Port      = 10000 + Client.Id
socks5.Remark    = Client.Remark
```

Manual add/edit/delete through `/index/*` is rejected for `type=socks5`;
create, update, or delete the client instead. `/index/start` and `/index/stop`
are allowed for `type=socks5` and persist the managed SOCKS open/close state.
Managed SOCKS tunnels are created closed by default. After opening one with
`/index/start?id=<client_id>&type=socks5`, it is automatically stopped and
persisted closed if its inlet/export flow does not change for 30 minutes.
