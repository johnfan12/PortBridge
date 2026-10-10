# 验证记录

2026-10-10，Linux amd64，Go 1.27.1。未修改主机系统 Go 或实际服务，所有验收使用临时目录与随机测试端口。

- `make check`：Go vet、全量竞态检测、JS/Bash 语法通过。
- 真实 HTTPS hub/node、回环 HTTP 与 WebSocket 后端：1,020,000 字节上传与回传、路径/查询/Host/Cookie 保留、WebSocket 双向消息通过。
- 配对只登记节点，在公网端口全部占用时仍可配对；并发兑换同一邀请仅一次成功，过期拒绝。
- 授权前无法创建或直连目标；维护端口不能授权；旧专用 SSH 类型被拒绝；配对链接不接受 NodeBridge 前缀。
- VPS 单代理与 node 全部 TCP 暂停在 1 秒内关闭已有连接、拒绝新连接；恢复、撤销授权、删除端口回收和删除节点清理均通过。
- 端口耗尽/占用失败回滚、重复目标拒绝、重启恢复固定端口与暂停、释放冲突后重试监听通过。
- 管理员权限、Cookie 会话、同源写保护、节点回环 Host 防护通过；旧 NodeBridge 数据库拒绝打开。
- Playwright 同时运行 PortBridge 与 NodeBridge 的 hub/node：两套管理员 Cookie 共存、错误项目配对拒绝、PortBridge 创建/打开模拟 Overleaf、暂停/恢复/撤销/删除，NodeBridge 仍可进行原 SSH 数据传输。
- 浏览器确认 NodeBridge 不再显示 TCP 面板、旧代理 API 404，PortBridge 只列出服务转发，没有自动 SSH 代理。自动刷新保留按钮焦点，创建后收起表单。
- 320/390 px 主界面（含展开创建表单）与本机授权页无横向溢出。截图在忽略的 dist/ui-hub.png、ui-node.png、ui-mobile.png。
- Linux amd64/arm64 Release 构建、压缩包内容与 SHA-256、amd64 版本号通过。

模拟 HTTP/WebSocket 服务验证了转发链路；实际 Overleaf 容器、多人编辑、公网网络与真实 arm64 运行仍需在目标机器验证。未执行真实系统安装/卸载，未安装 Nginx 或申请证书。

首次正式发布使用 v0.1.1；初始源码标签 v0.1.0 未触发新仓库的发布任务，main 的 GitHub 检查已通过。
