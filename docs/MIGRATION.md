# 从 NodeBridge 拆分

PortBridge 从 NodeBridge v0.2.0 的 TCP 服务转发实现拆分，采用独立 Go 模块、程序、服务、WebUI、安装器、认证与数据目录。NodeBridge v0.3.0 起只管理 SSH。PortBridge 不是 FRP 协议兼容实现，也不需要部署 frps/frpc。

## 迁移现有 Overleaf

1. 备份旧 NodeBridge hub/node，记录各代理的节点、名称、本机端口、公网端口和暂停状态。
2. 并行安装 PortBridge hub/node，使用默认 9543/9900 与 40000–49999，先保留旧转发。
3. 在新 hub 生成新链接，用 `portbridge pair` 配对，授权原服务的回环端口，创建新转发。
4. 放行新公网端口，验证 Overleaf 登录、上传、编辑与 WebSocket；更新服务自己的站点地址和用户访问书签。
5. 验证完成后升级 NodeBridge 至 v0.3.0，原 SSH 配置继续使用，旧额外 TCP 转发停止。

若必须沿用旧公网端口，先释放原 NodeBridge TCP 监听，确保 PortBridge 端口池包含该端口，再指定端口创建；已有代理改端口需删除并重新创建。过程会中断该服务连接，不能同时由两个程序监听同一端口。

不要复制旧数据库、证书和配对凭证到新目录。PortBridge 配置和数据库带有独立项目标记，误用 NodeBridge 数据会拒绝启动。旧版 NodeBridge 代理元数据在 v0.3.0 仅作为迁移参考保存；恢复旧版时可能重新开放旧转发，回滚应整套恢复备份并处理端口冲突。
