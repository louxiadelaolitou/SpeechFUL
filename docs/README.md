# 开发文档

SpeechFUL 是一个多端仓库：Flutter 手机 App、静态审核工作台和官网，以及共用 Go 模块内的社区 C／校邮验证 V 两个服务。

## 从哪里开始

| 目的 | 入口 |
| --- | --- |
| 熟悉目录、页面和接口调用 | [代码导览](development.md) |
| 本地构建、测试与验证分层 | [开发与验证](development.md#开发与验证) |
| App 配置与运行 | [移动端 README](../apps/mobile/README.md) |
| C/V 初始化与运行 | [API README](../services/api/README.md)、[运行入口](operations/README.md) |
| 审核工作台／官网 | [admin](../apps/admin/README.md)、[website](../apps/website/README.md) |
| 产品术语与规则来源 | [唯一术语表](../GLOSSARY.md)、[设计索引](design/README.md) |
| HTTP 契约与固定协议样例 | [OpenAPI](../packages/openapi/README.md)、[认证向量](../packages/auth-protocol-vectors/v1.json)、[帖子向量](../packages/post-protocol-vectors/README.md) |
| 工程决策 | [ADR](adr/) |
| 开发协作 | [AGENTS.md](../AGENTS.md)、[CONTRIBUTING.md](../CONTRIBUTING.md) |
| 近期工作与具体任务 | [RECENT_WORK](RECENT_WORK.md)、[任务记录](tasks/) |
| 旧讨论、设计与验收资料 | [历史追溯](archive/README.md) |

## 怎样判断信息是否适用

- 当前需求以对应任务中已经确认的取舍为依据，术语集中维护在 `GLOSSARY.md`；较新的明确变更替代同一范围的旧规则。
- 设计说明用于解释规则与边界，源码和配置用于核实实现。发现两者不符时先核对确认依据，不通过文档整理改变业务。
- 实现、验证、合并和发布分别查看任务记录；旧报告的 PASS 只证明其记录的提交、环境和范围。
- 日期化报告、截图和 JSON 回执是历史证据，不是日常操作指令或机器现状。被替代的交接台账与专用校验通过固定 Git 提交追溯，不再维护第二份当前状态。
