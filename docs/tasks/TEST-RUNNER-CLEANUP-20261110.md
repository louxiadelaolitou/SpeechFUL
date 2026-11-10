# TEST-RUNNER-CLEANUP-20261110 — 清理一次性和过时验收脚本

- 模式：常规；负责人／状态维护人：当前 Session。
- 分支／Worktree／PR：`codex/chore/retire-obsolete-test-runners`；独立 Worktree `SpeechFUL-test-cleanup-20261110`；基线为实际 fetch 后的 `origin/main`：`fbf5e6be302432db8a187a6293403b88be454bc7`；未创建 PR。
- 授权依据与边界：用户明确要求“把这些一次性、重复、过时的测试脚本清理掉，正常的功能测试保留，别影响现有功能。然后直接推送提交到 main 上”。授权本任务清理、提交及直接更新远端 main；不授权混入其他分支重构、修改产品行为、部署、操作数据库或设备。不强制推送。
- 需求确认：删除以固定历史运行、失效路径或被当前入口替代为依据；不以曾经 PASS、文件名旧或数量多为删除依据。保留当前功能测试、真实 SQL／设备行为验证、CI 和现行 runner 的依赖。无未决产品取舍。
- 更新日期：2026-11-10。
- 状态：实现 已完成；验证 PASS（静态依赖与主机工具范围；3 项 POSIX 场景 SKIP）；合并 待推送；发布 未发布。

## 目标与范围

减少当前树中不能用于新版本的历史执行脚本，保留可复用测试及运行边界。历史文件从固定基线 Git 读取，不复制到新的归档目录，不重写既有 JSON 证据、摘要或历史 PASS。

## 删除依据

| 删除组 | 范围 | 已核实依据 |
| --- | --- | --- |
| 2026-10-15 Passkey 实验执行归档 | `tools/community-android-stage3/execution-20261015/` 的 8 个 `.py/.sh/.dart` 文件；保留历史入口 README | 固定当次目录、owner、lab、源码 SHA；Dart 文件需由旧 wrapper 人工复制进镜像后才运行，未被当前测试发现或 CI 调用。删除整个一次性内部调用闭包，保留当前 Stage 3 准备工具及其守卫测试、App 和后端认证功能测试。 |
| P5-D1 一次性评论性能观测 | `tools/comment-snapshot-process/p5d1_{analyze,clock_observe,master,prepare,verify}.py` 共 5 个文件 | 固定 2026-10-24 旧环境和单次 overlay／输出；master 还依赖不在当前仓库中的冻结驱动。无当前 runner／CI 调用。保留 campaign、兼容、故障、容量、长 TTL、回退和可复用性能 runner。 |
| 图片媒体旧证据生成和清理 | `tools/post-image-media/record_evidence.py`、`cleanup_owned.py` | 固定 2026-10-15 任务和证据文件，前者写死旧分支／版本／测试包数量和结果解释，后者仅清理旧任务目录。不作为产品测试；保留协议向量和 OpenAPI 工具。此次只删除脚本，不执行任何清理命令。 |
| 被替代的 F3 主机 wrapper | `tools/run-community-flutter-f3-linux.sh` | 固定旧 F3 owner 和 SDK 默认目录，仅复制旧依赖子集，缺少当前本地包。当前 Flutter 直接命令与仍被实际集成测试引用的 F4 runner 已覆盖其用途；没有代码或 CI 调用旧 F3 wrapper。 |

共 16 个脚本。没有为了凑删除数量合并仍验证不同边界的测试。

## 验收标准

- 剩余可执行代码和 CI 不引用已删除脚本；当前快照 campaign 的本地模块及动态脚本依赖完整。
- 本次新增／受影响的 Markdown 链接有效；历史来源可从固定基线读取；既有 JSON 和字节归档保持不变。
- 产品源码、全部正常功能测试、正式迁移／权限 SQL、锁文件和 CI 与基线相同。
- 运行适用主机工具测试、协作检查和差异检查；已有环境失败与新增问题分开记录。
- 推送前重新 fetch，确认远端 main 是本次提交的祖先；不覆盖其他 Session 的提交或工作区。

## 当前进度与证据

| 项目 | 环境／版本 | 结果与证据 |
| --- | --- | --- |
| 基线与隔离 | fetch 后 `fbf5e6b`；Windows PowerShell | PASS：`D:/SpeechFUL` 在另一条重构分支上，本任务使用独立 Worktree，未切换其分支或修改其文件。 |
| 删除调查 | 基线文件、调用引用和 CI | PASS：按上表核对闭包；保留正常 Go／Flutter／原生测试及现行 runner。 |
| 基线／清理后对照 | `fbf5e6b`／本任务最终差异；原 Windows Python 3.9.13；`python -B -X utf8 -m unittest discover -s tools/tests -v` | 两次均为 197 PASS、3 SKIP、1 BLOCKED：原环境缺少 `openapi_schema_validator`，没有新增失败；不将其计为全套通过。 |
| 完整工具回归 | 本任务独立 Python 3.12 虚拟环境，按 `packages/openapi/requirements-check.txt` 安装已声明依赖；`python -B -X utf8 -m unittest discover -s tools/tests -v` | PASS：201 项中 198 PASS、3 SKIP，exit 0。SKIP 为 Windows 不具备 POSIX 符号链接／权限语义的既有场景。依赖只安装于任务临时环境，不改全局 Python、仓库锁文件或产品依赖。 |
| 功能与依赖保留 | `git diff --exit-code fbf5e6b -- apps services packages infra .github`；剩余代码／CI 的完整删除路径及唯一文件名引用扫描、Python AST 检查 | PASS：产品源码、363 个功能测试／夹具文件、正式 SQL／权限、锁文件及 CI 无差异；无可执行文件引用删除项；127 个剩余工具 Python 文件可解析。当前快照 campaign 的 8 个静态／动态子脚本均存在。 |
| 文档与历史 | 检查 2884 个本地 Markdown 链接及固定历史 Git 路径 | PASS：没有本轮新增或因删除产生的断链；全部新历史入口存在。另识别 162 个基线已有的历史讨论／字节归档等缺失资源链接，保持原状，不把它们计为本轮新增问题或改写原始档案。 |
| 产品运行验收 | 本轮未变产品实现 | NOT_RUN：未启动／更新公网服务、数据库或设备，未复跑历史 SQL／真机验收。 |
| 协作与差异 | 最终暂存区；`python tools/check-collaboration.py --staged`、`git diff --cached --check` | PASS：仅包含 16 个脚本删除和 7 个 Markdown 文件更新／新增，未混入其他任务。工具回归已包含全部 19 项协作规则测试。 |
| 合并和推送 | 待验证 | NOT_RUN |

## 阻碍与下一步

删除、依赖、主机工具和协作检查完成。重新 fetch 后按明确授权正常推送到 main；不部署。其余保留的历史名称 runner 仍有实际功能测试、CI 或依赖用途，未按名称批量删除。
