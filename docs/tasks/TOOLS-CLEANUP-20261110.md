# TOOLS-CLEANUP-20261110 — 清理 tools 单次阶段材料

- 模式：常规；负责人／状态维护人：当前 Session；无委派。
- 分支／Worktree／PR：`codex/chore/tools-cleanup-20261110`；`E:/Hnuhole-env/source/SpeechFUL-tools-cleanup-20261110`；[PR #128](https://github.com/zewbby/SpeechFUL/pull/128)。初始基线 `fbf5e6be302432db8a187a6293403b88be454bc7`，工作区干净；期间 main 合入前一项 runner 清理，已通过 `f2b72a1` 整合 `e9293f1cfd386fff24772445c894c54cdf686a5b`，最终差异以该 main 为准。
- 授权依据与边界：用户先要求梳理 main 上 tools 中已无需要的阶段产物，随后明确“弄好以后直接清了合并上去就行”。覆盖调查、删除已确认阶段材料、必要文档与引用修复、本地提交、推送、PR 及检查通过后合并 main；不包含部署或清理外部设备、服务、数据库、凭据与缓存。
- 需求确认：清理单次批次的实验／交付／机器辅助源码，通过固定 Git 提交追溯；仍支撑现行 CI、可复用验证及安全／设备行为覆盖的工具保留。用户已明确授权清理与合并，无未决产品取舍。本轮不改变产品行为。
- 更新日期：2026-11-10。
- 状态：实现 已完成；验证 本地适用检查及远端协作检查 PASS（跳过项与证明范围见下）；合并 [PR #128 实时状态](https://github.com/zewbby/SpeechFUL/pull/128)；发布 未发布。

## 目标与范围

从 tools 移除已结束实验和固定旧机器的独立脚本，补充工具用途导航，历史文档链接改为固定提交。前一项[runner 清理](TEST-RUNNER-CLEANUP-20261110.md)已移除 16 个重叠脚本并合入 main；保留其历史入口与任务记录，本 PR 只补清剩余 9 项，不重复统计。

## 删除依据与依赖

| 删除项 | 数量 | 依据 |
| --- | --- | --- |
| `post-image-media/build_openapi.py` | 1 | 初次落地图片 API 的源码编辑工具；会重写规范，尚含旧的 Body／DTO 更新逻辑，不是当前规范校验或构建依赖；保留现行规范、校验器、协议向量生成器及向量原值 |
| `control-android-b3-b4-provider.py`、`diagnose-android-b3-b4-selected.py`、`keep-android-b3-b4-environment.py` | 3 | 已结束 Google 提供方／选中回调诊断和旧 WSL 服务保活，固定旧设备／私有批次目录，无保留源码或 CI 调用 |
| `operate-android-b3-b4-isolated.py`、`reveal-android-b3-b4-target.py`、`update-android-b3-b4-candidate.py`、`wait-android-b3-b4-native.py` | 4 | 固定旧机器矩阵与包名的独立安装／选中／等待辅助；保留 fresh 构建／操作器、原生诊断和签名／未知结果等回归，不移除其依赖 |
| `restore-auth-machine-archive.py` | 1 | 旧机器迁移专用加密缓存恢复格式，现行开发／候选／服务恢复流程无调用；不处理归档或密钥本体 |

最终相对 `e9293f1` 新删除 9 个文件，Git blob 原大小 50,918 字节；与前项共 25 个脚本。初始调查覆盖 26 个文件，整合时保留前项新建的执行归档 README，将另外 16 个重复删除从本轮统计剔除。当前源码减少量不代表 Git 历史或外部缓存体积减少。剩余日期化工具因可复用行为、调用与边界验证保留，见 `tools/README.md`。原始历史 JSON 和证据摘要不改写。

## 验收标准

- 保留运行源码、CI、hook 不再引用删除路径或导入删除模块；历史 Markdown 链接指向可读取的固定 Git 对象。
- `apps/`、`services/`、`packages/`、`infra/`、`.github/`、现有行为测试和锁文件无差异。
- 现有 tools 适用回归、契约检查、差异空白及协作检查通过；缺环境的验证单列，不冒充真实设备或 SQL 验收。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 调查与隔离 | Windows；main `fbf5e6b` | PASS：已核对实际仓库、remote、worktree、基线及依赖；独立任务分支，不改已有工作区 |
| 删除与历史入口 | `e9293f1`＋本任务最终差异 | 新删除上述 9 项；历史 Markdown 补充退役说明；前项历史入口完整保留，Git 保留原始内容 |
| 主工具回归 | 基线＋本任务最终差异；Windows 验证 venv／Python 3.12.14；`python -B -X utf8 -m unittest discover -s tools/tests -v` | PASS：201 项，198 PASS／3 SKIP（POSIX 权限要求）；全部现有测试保留 |
| 其他 Windows 适用工具 | 相同版本；`python -B -m unittest discover -s <目录> -p 'test_*.py' -v`，目录为 `tools/android_integration/tests`、`tools/ci/tests`、`tools/android-passkey-diagnostic` | PASS：分别 8 项（7 PASS／1 SKIP）、4 PASS、8 PASS |
| Linux 专用工具 | WSL `HnuHole-Ubuntu-24.04`／用户 hnuhole／Python 3.12.3；`python3 -B -m unittest discover -s <目录> -p 'test_*.py' -v` | PASS：`development_runtime/tests` 11、`feed_staging` 22、`smtp/tests` 4、`post-media-android` 15；`prerelease/tests` 16 项（9 PASS／7 SKIP：未启用真实 PostgreSQL 审计）。媒体准备的 `POST_MEDIA_PREP_TEST_ROOT` 为本任务临时 Linux 目录，测试后仅清除该目录 |
| 环境不适用的初次尝试 | Windows 额外工具套件 | 初次 FAIL 归因为 Linux `fcntl`／POSIX 权限与媒体测试缺少临时根参数；改在上述 WSL 环境与临时根后适用测试全部 PASS。未修改工具或测试以绕过环境限制 |
| 规范校验 | Windows 验证 venv；`python -B -X utf8 packages/openapi/check-specs.py` | PASS：13 份现行 OpenAPI／契约，规范与向量无差异 |
| 引用与结构 | 初始 `597bea0` 及整合后的差异；一次性 Python 扫描全部保留执行源码／CI／hook、Python AST、修改 Markdown 路径和固定 Git 对象 | 初始 PASS：无删除项运行引用、147 份 Python AST、83 个本地 Markdown 链接、10 个固定 Git 路径。`git diff --name-only 597bea0 f2b72a1` 仅有 Markdown，整合不改变被测可执行内容；最终引用复核见下。产品源码、CI、现有测试与锁文件无差异 |
| 整合后复核 | `e9293f1`＋本任务最终差异；相同依赖／AST／Markdown 与历史对象检查 | PASS：9 个删除项无运行引用或导入；147 份 Python AST、69 个本地链接、4 个固定 Git 对象；产品／CI／现有测试无差异。`597bea0` 的可执行树与最终版本相同，以上工具回归证据继续适用 |
| 差异与协作 | `git diff --check`、`git diff --cached --check`；提交前 `python tools/check-collaboration.py --staged` | PASS：空白检查及暂存协作检查均通过 |
| PR 与远端 CI | `79bda674495f1a7a56c0b31f71c192821e95cddd`；[Collaboration 42821848721](https://github.com/zewbby/SpeechFUL/actions/runs/42821848721) | PASS：completed/success；最终差异只触发协作 workflow。无产品行为或 CI 配置变更；不将协作 CI 当作产品验收。后续仅补本记录，当前 head 检查以 PR Checks 为准 |
| 产品／设备／SQL 验收 | 无产品实现变更 | NOT_RUN：本轮不启动设备、业务服务或数据库，不重写历史结果 |

## 阻碍与下一步

本地适用检查完成；保留套件共 289 项，278 PASS／11 SKIP。真实 PostgreSQL 审计、设备及产品部署未执行；本次仅删除无运行引用的阶段工具，不改变这些边界的实现和现有测试。已推送并创建 PR，远端适用检查通过；按用户明确授权合并，最终合并结果与 head 检查以 PR 实时记录为准。本任务无需部署，无产品取舍阻碍。
