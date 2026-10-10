# 运维

## 服务和诊断

```bash
systemctl status portbridge-hub
systemctl status portbridge-node
journalctl -u portbridge-hub -f
journalctl -u portbridge-node -f
curl http://127.0.0.1:9900/api/health
curl http://127.0.0.1:9900/api/node
```

配置与数据为 `/var/lib/portbridge/<mode>`；节点只允许回环维护访问。远程管理节点维护页可用节点原有 SSH 隧道，或使用本机 curl API，不能将维护页绑定公网。

连接失败先检查 VPS 控制台端口、节点日志与 TLS 指纹；更换证书后需新配对。配对链接单次消费、15 分钟有效；凭证撤销后需删除 hub 旧节点，再停 node、备份并移除其 config.json 后启动重新配对，恢复本机授权。

## 服务端口不可用

- 节点离线：检查到 hub 的连接。自动重连采用 1–30 秒退避。
- 端口未授权：在本机维护页添加实际端口。
- 服务未就绪：先 `curl http://127.0.0.1:端口`，检查容器宿主机端口和服务监听。
- 已暂停：检查 VPS 单服务与 node 全部 TCP 两个独立开关。
- 监听冲突：停止公网端口占用进程后，点“重试监听”或“恢复转发”。重启不会偷偷更换已分配端口。

就绪表示 node 与本机服务可达，不证明外部防火墙已放行。VPS 云安全组需开放所分配端口。状态每 10 秒上报，授权与暂停触发及时上报；界面每 5 秒刷新。

HTTP/WebSocket 直接通过；Overleaf 登录与对外地址由其自身负责。当前不自动终止 TLS 或安装 Nginx。自行添加反向代理时将代理目标指向 VPS 分配的本地端口并支持 WebSocket。

## 本机 API

所有写请求要求 JSON 与 `X-PortBridge-Request: 1`，例：

```bash
curl -fsS -X PUT http://127.0.0.1:9900/api/tcp-forwarding \
  -H 'Content-Type: application/json' -H 'X-PortBridge-Request: 1' -d '{"paused":true}'
# 恢复改为 paused:false
curl -fsS -X DELETE http://127.0.0.1:9900/api/tcp-ports/8080 \
  -H 'Content-Type: application/json' -H 'X-PortBridge-Request: 1'
```

暂停/撤销立即断开连接，不停止 Overleaf。完整关闭 Web/后端使用 `sudo systemctl stop portbridge-node`（VPS 改 hub），恢复使用 start。

## 备份、更新、卸载

停 hub 后备份整个 hub 目录，包含 hub.db、config.json、tls.pem、tls.key；node 备份 config.json。证书与数据库需一并恢复。恢复归属 portbridge:portbridge，目录 0700、敏感文件 0600。

更新重复执行 README 的安装命令；已有状态保留，重启断开连接。卸载默认保留数据，带 --purge 永久删除；仅操作 portbridge 相关服务、账号和目录。同机 NodeBridge 保留。
