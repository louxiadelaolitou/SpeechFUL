# 推荐核心交接与剩余验收

- 更新日期：2026-11-27
- 分支：`codex/feat/recommendation-mainline`（Worktree `../speechful-recommendation`），基线 `develop/main@93deaf1`；移植提交 `f7dfd28`（cherry-pick 原交接提交 `4d72156`，推荐核心文件树一致）。
- 原交接分支：`codex/feat/recommendation-core-handoff`，基线 `bc599b342da707169e55ddff90e8f2a54fc751cc`，保留供追溯。
- PR：[#43](https://github.com/zewbby/SpeechFUL-Develop/pull/43)，目标 `develop/main`；代码 head `165652b` 的 CI PASS（18 success、1 skip）。
- 状态：推荐核心已移植到当前主线并通过本地聚焦验证；PR 已创建。服务端整合、正式验收、合并和发布尚未完成。
- 授权边界：2026-11-26 指令“拉到本地，以当前 main 分支为基线继续接入主线，进行测试”，授权本地移植与测试；2026-11-27 指令“提pr”，授权推送分支并创建 PR。合并、发布、部署仍未授权。

## 本次交付

- [推荐核心](../../services/api/internal/recommendation/README.md)提供四路候选选择、合并排序和回流去重。输入包括候选、时间、种子和历史，因此可以用固定数据重放排序。
- [轮次模型](../../services/api/internal/recommendation/roundmodel/README.md)固定每轮最多 100 个唯一帖子、每页 10 槽，覆盖同意图重试、曝光反馈、到期清理和容量预留。模型上限为每账号 1,024 条下发历史和 2,048 条曝光历史，默认曝光窗口为 72 小时。
- `transport` 提供推荐上下文、游标和反馈回执的有界编解码。`recommendationlab` 提供 30 组固定合成样本，可比较去重、回流、新帖覆盖和来源分布。

## 服务端接续入口

上一轮接线保存在原工作树 `HnuHole-recommendation-completion-20261108` 的 `codex/fix/recommendation-cleanup-throughput`，即旧基线 `856313ccb800f6786502d4ddfb13678b0bfe9b2e` 加本地改动。入口为 `services/api/internal/authprivacy/community_recommendation*.go`、`internal/authprivacyhttp/recommendation_endpoints.go` 和 `internal/authprivacyruntime/recommendation_runtime.go`。原任务记录在该工作树的同名文件，脱敏结果位于原工作区 `feed-comment-performance-20261025/AB-B-REMAINING/verification-20261109/`。

2026-11-26 在本机核对时，该工作树与 `codex/fix/recommendation-cleanup-throughput` 分支均已不存在；对象库仅有旧基线提交 `856313ccb800f6786502d4ddfb13678b0bfe9b2e`，本地改动缺失。恢复接线前需先找回该源码，或按当前主线重写。

接入当前主线时，从届时的主线分支另行移植 PostgreSQL、HTTP 和运行时接线。主线已使用旧推荐迁移编号 0028–0030，且已有 0044 Gate checkpoint 及后续迁移。重新分配可用迁移编号，按当前 Gate、Feed 投影、共享限流和最小权限约定逐项适配，包括升级库回归。默认关闭推荐，保留既有最终事务授权、账号／会话绑定、配额和未知写处理。

## 验证和待办

| 项目 | 结果 | 范围和依据 |
| --- | --- | --- |
| 主线移植的纯 Go 核心 | PASS | Windows 11 25H2 x64、Go 1.22.2 windows/amd64、`GOMAXPROCS=2`、`GOPROXY=off`。`go test -mod=readonly -count=1 ./internal/recommendation/... ./cmd/recommendationlab`（4 个包通过，CLI 无测试文件）、`go build`、`go vet`、Go 文件 `gofmt` 与 `git diff --check` 均通过。离线样本连续运行两次字节一致，SHA-256 同为 `673f00e2d21d16c376b3bc4f42b0519c8259bca0b004b8ce9d6a6f71e0652dea`。 |
| PR #43 CI | PASS | 代码 head `165652b`；19 项检查 18 success、1 skip（`actual-process-acceptance` 按设计跳过），无 failure。 |
| 全仓 Linux 编译 | PASS | `GOOS=linux GOARCH=amd64 go build -mod=readonly ./...` 与 `go vet -mod=readonly ./...` 通过，对应产品 CI 的编译范围。 |
| 全仓 Windows 测试 | BLOCKED | `go test ./...` 在既有 `internal/authprivacy/authorization_evidence.go` 的 `syscall.Flock` 处构建失败；`develop/main` 基线同一位置已存在，属 Windows 平台限制，与本移植无关。 |
| 本地 race 检测 | NOT_RUN | 本机无 C 编译器，`go test -race` 返回 `-race requires cgo`；原交接提交同内容已在 WSL2 Go 1.22.2 记录 race PASS。 |
| 原分支纯 Go 核心 | PASS | WSL2 Linux、Go 1.22.2，运行模块 README 中的聚焦 race、build、vet 及格式检查。离线样本连续运行两次，输出字节一致，SHA-256 为 `673f00e2d21d16c376b3bc4f42b0519c8259bca0b004b8ce9d6a6f71e0652dea`。 |
| 上一轮隔离 PostgreSQL | PASS | 旧集成树的定向回归覆盖 72 小时曝光保留、2048 上限、候选补位、批量到期清理、最终冻结和撤销围栏。证据见 `capacity-r12-06d49da6a1d5a24d/validation-r12/receipt.json`，仅适用于记录的旧源码。 |
| 原生 J 整请求 SQL／P95 | NOT_RUN | 已准备独立采样脚本。按 20 次热身及每项 200 次正式样本检查建轮 SQL ≤64、翻页／重试／反馈 SQL ≤40；建轮 P95 ≤350 ms、翻页／重试／反馈 P95 ≤180 ms。 |
| 本机 300 账号一小时 | 校准 FAIL，小时 NOT_RUN | 原生 J 隔离副本已备 300 账号和 2,001 条混合帖。2026-11-09 的安全容量校准为 247,252 工作单位，低于目标 2,059,200，准入前正确拒绝。小时负载实际为 0 次请求。证据见 `capacity-r12-06d49da6a1d5a24d`。 |

继续验证前先完成新主线接线，并复验空闲账号与小批尾部的清理耗时。以本机原生 J 隔离副本重新校准，保留原 J 和此前结果。容量通过后，在共享出口 IP 下用 300 个账号持续 60 分钟，每人每 30 秒加载 10 帖并上报全部 10 次曝光，每 5 分钟建一次最多 100 帖的轮次。核对到限拒绝、实际曝光保留、清理积压和 Gate 状态。真实用户收益仍未实测。

截至 2026-11-27，纯推荐核心已移植到 `develop/main@93deaf1`，本地聚焦检查与 PR #43 CI 均通过；服务端接线尚未开始，下一步先恢复上一轮接线源码，再按当前迁移编号、Gate、Feed 投影和共享限流约定适配。
