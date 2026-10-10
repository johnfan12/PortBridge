# PortBridge

- 独立 Go 仓库，一个 portbridge 二进制，hub/node 双模式；从 NodeBridge v0.2.0 的 TCP 转发拆分。
- 负责 Overleaf 与明确授权的本机回环 TCP 服务；不提供专用 SSH 连接、SSH 命令、GPU 或计费面板。
- 配对只登记节点，不自动监听业务端口；管理员创建转发时事务分配公网端口。
- 正式仓库 https://github.com/johnfan12/PortBridge，安装器默认下载该仓库最新 Release。
- 默认 hub 9543、node 9900、服务端口池 40000–49999；独立 portbridge 用户、单元、配置、数据库、Cookie 和 portbridge:// 配对链接。
- 节点维护页只监听回环；目标仅为本机允许列表，禁止转发维护端口；写操作保留 Host/Origin/请求头保护。
- 账号与节点凭证独立；安装时生成管理员，不允许首个公开注册者成为管理员。
- 配置原子保存，数据库使用事务。暂停/撤销授权需关闭相应的活动连接。
- 核心变动运行 make check；发布运行 make release VERSION=<version>。同步更新 README、迁移和运维文档。
- 不自动部署 Nginx 或 ACME，不修改现有 Overleaf、NodeBridge 或系统 SSH 服务。
