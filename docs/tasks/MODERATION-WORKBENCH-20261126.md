# MODERATION-WORKBENCH-20261126 — 审核工作台值班队列完善

- 模式：常规；负责人／状态维护人：当前 Session。
- 分支／Worktree／PR：`codex/feat/moderation-workbench-refinement`；`E:/code_collection/java_code/SpeechFUL-moderation-workbench`；[PR #41](https://github.com/zewbby/SpeechFUL-Develop/pull/41)（draft）。
- 基线：原始功能基线 `develop/main@93deaf1`；追加修复已同步 `develop/main@f6b2419`。
- 授权依据与边界：用户要求“建立分支。完善审核平台，先提出设想然后进行实现，验证后PR”，随后明确要求继续修改现有 PR、验证后再提 PR，并反复强调不得合并。授权覆盖本任务实现、本地提交、推送分支和维持 draft PR；不包含合并或部署。
- 需求确认：用户确认待处理优先的值班队列，并在自查后要求全力修复。追加修复范围为本人降级空白页、队列失败残留加载态、领取后队列空窗、移动端触控尺寸和厚色条视觉问题。未增加批量处理、导出、账号身份信息或其他治理规则。
- 更新日期：2026-11-27
- 状态：实现已完成；本地验证 PASS；PR CI PASS；合并未合并；发布未发布。

## 目标与范围

把静态审核工作台完善为连续值班时可快速定位、核对和提交的工作区。包含：

- `/api/v1/moderation/cases` 增加可选 `state` 与精确 `number` 筛选，先过滤再由服务端游标分页。
- 前端案件队列默认待处理，提供状态分段控件、工单号搜索、刷新、加载更早记录和上下键／Home／End 切换。
- 处理表单展示“本次将执行”的明确后果；审计失败原位重试；筛选后案件状态变化时保留详情并说明已移出队列。
- 视觉层级、焦点态、状态色和 390px 窄窗适配；保持四文件静态发布、同源 CSP 和零第三方资源。
- 追加修复：本人降级回退案件页、队列失败进度一致、领取后上下文卡、移动端 44px 触控尺寸和厚侧边色条。

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
- 队列首次失败后显示明确失败进度，点击重试恢复筛选结果。
- 管理员本人降为审核员后回到“举报案件”，不留在已隐藏的权限页。
- 领取案件后左侧显示当前案件上下文，并可切换到对应状态查看。
- 390px 下无横向溢出，所有可见按钮、输入框和选择框至少 44px 高；登录、退出、角色回避、未知结果重试和隐私边界保持。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | `a146277` 后追加修复差异 | 已完成本人降级、失败进度、领取上下文、移动端触控和提示样式修复；PR #41 保持 draft |
| Node 与隐私契约 | Node 24.21.0；`node --check`、`node --test apps/admin/test/core.test.mjs` | PASS：16 项，覆盖筛选查询、失败进度、操作摘要、移动端尺寸约束、未知结果与隐私边界 |
| 合成 HTTPS 浏览器 | Playwright 1.62.1、Chrome、OpenSSL 3.5.4；一次性本机 HTTPS 夹具 | PASS：8 次业务命令、0 页面错误、0 外部请求，覆盖队列失败重试、本人降级回退、领取上下文、筛选／搜索／审计重试／键盘、1440px 与 390px |
| OpenAPI | Python 3.12、PyYAML 6.0.1、openapi-spec-validator 0.9.0 | PASS：13 份规范完整校验 |
| Go 编译 | Go 1.26.2；`GOOS=linux go vet`、`go build ./cmd/...`、`GOOS=linux go test -c` 两个受影响包；`internal/moderation` 本机测试 | PASS：静态检查、命令构建、Linux 测试二进制编译与 moderation 包测试通过 |
| 协作检查 | 暂存差异；`python tools/check-collaboration.py --staged`、19 项协作测试 | PASS |
| 真实 Linux SQL／HTTPS | PR #41 head `7e97938`，[Report moderation run 45969527869](https://github.com/zewbby/SpeechFUL-Develop/actions/runs/45969527869) | PASS：`admin` 与 `moderation-postgres` 均成功；真实 PostgreSQL、权限、HTTP 与浏览器工作台回归 PASS |
| 视觉检测 | Impeccable detector，`app.mjs`、`styles.css`、`index.html` | PASS：0 条命中；厚侧边色条已移除 |

## 阻碍与下一步

- 本机缺少 Linux runtime；最终修复 head 的 `Report moderation` 已由 Linux CI 复验通过。
- 下一步：PR #41 保持 draft 交给管理员；合并与部署仍需另行授权。
