# MODERATION-WORKBENCH-20261126 — 审核工作台值班队列完善

- 模式：常规；负责人／状态维护人：当前 Session。
- 分支／Worktree／PR：`codex/feat/moderation-workbench-refinement`；`E:/code_collection/java_code/SpeechFUL-moderation-workbench`；[PR #39](https://github.com/zewbby/SpeechFUL-Develop/pull/39)。
- 基线：`develop/main@93deaf1`。
- 授权依据与边界：用户要求“建立分支。完善审核平台，先提出设想然后进行实现，验证后PR”，并确认“验证后PR”。授权覆盖本任务实现、本地提交、推送分支与创建 PR；不包含合并或部署。
- 需求确认：用户确认待处理优先的值班队列。案件默认“待处理”，可切换处理中／已结束／全部；按完整数字工单号精确搜索；队列支持键盘切换；提交前展示操作后果；操作记录可重试。未增加批量处理、导出、账号身份信息或其他治理规则。
- 更新日期：2026-11-26
- 状态：实现已完成；验证 PASS；合并未合并；发布未发布。

## 目标与范围

把静态审核工作台完善为连续值班时可快速定位、核对和提交的工作区。包含：

- `/api/v1/moderation/cases` 增加可选 `state` 与精确 `number` 筛选，先过滤再由服务端游标分页。
- 前端案件队列默认待处理，提供状态分段控件、工单号搜索、刷新、加载更早记录和上下键／Home／End 切换。
- 处理表单展示“本次将执行”的明确后果；审计失败原位重试；筛选后案件状态变化时保留详情并说明已移出队列。
- 视觉层级、焦点态、状态色和 390px 窄窗适配；保持四文件静态发布、同源 CSP 和零第三方资源。

不改变处罚、申诉、证据、通知、角色或最终 C Safety Gate 规则。

## 接口与依赖

- `GET /api/v1/moderation/cases` 新增可选 `state=PENDING|PROCESSING|ENDED` 与 `number=<canonical positive int64>`；非法值返回 `400 MALFORMED_REQUEST`。
- 原有 `before`、`limit`、权限、DTO 与详情接口不变；无数据库迁移。
- `packages/openapi/moderation-api.yaml`、管理端 README 与治理规则同步。

## 验收标准

- 默认筛选只返回待处理案件；切换状态和精确工单号后，列表、空状态、游标续页与请求参数一致。
- 选择案件后详情、授权证据、处理表单和审计对应同一记录；上下键可切换；筛选变化清除旧详情。
- 处理措施与期限变化时，操作后果摘要和最终提交字段一致。
- 审计首次失败可原位重试；迟到的材料页仍不能串入其他案件。
- 390px 下无横向溢出；登录、退出、角色回避、未知结果重试和隐私边界保持。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | `ead5b71` | 已完成接口、前端、测试和文档修改并推送 PR #39 |
| Node 与隐私契约 | Node 24.21.0；`node --check`、`node --test apps/admin/test/core.test.mjs` | PASS：15 项，覆盖筛选查询、操作摘要、未知结果与隐私边界 |
| 合成 HTTPS 浏览器 | Playwright 1.62.1、Chrome、OpenSSL 3.5.4；一次性本机 HTTPS 夹具 | PASS：7 次业务命令、0 页面错误、0 外部请求，覆盖筛选／搜索／审计重试／键盘、1440px 与 390px |
| OpenAPI | Python 3.12、PyYAML 6.0.1、openapi-spec-validator 0.9.0 | PASS：13 份规范完整校验 |
| Go 编译 | Go 1.26.2；`GOOS=linux go vet`、`go build ./cmd/...`、`GOOS=linux go test -c` 两个受影响包；`internal/moderation` 本机测试 | PASS：静态检查、命令构建、Linux 测试二进制编译与 moderation 包测试通过 |
| 协作检查 | 暂存差异；`python tools/check-collaboration.py --staged`、19 项协作测试 | PASS |
| 真实 Linux SQL／HTTPS | PR #39 产品实现提交 `ead5b71`；文档头 `64818da` 复跑，[Report moderation run 45787491266](https://github.com/zewbby/SpeechFUL-Develop/actions/runs/45787491266) | PASS：`admin` 与 `moderation-postgres` 均成功；新 `TestModerationCaseFilter`、`TestModerationCaseListFiltersPostgres` PASS，真实 PostgreSQL、权限、HTTP 与浏览器工作台回归 PASS |

## 阻碍与下一步

- 本机缺少 Linux runtime，但对应真实测试已由 PR #39 的 `Report moderation` CI 在 Linux `moderation-postgres` 作业中执行并通过。
- 下一步：本记录更新后如仅文档变化，不改变上述产品 CI 的证明范围；PR 可转为正式送审。合并与部署仍需另行授权。
