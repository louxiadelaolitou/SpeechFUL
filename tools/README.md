# 开发与验证工具

本目录保留当前契约检查、开发运行、候选构建和可复用的受控验收工具。带日期或阶段名不代表可以直接删除：部分脚本仍提供签名隔离、设备归属、未知结果处理和真实进程回归，必须结合调用与行为覆盖判断。

## 按用途查找

| 用途 | 入口与边界 |
| --- | --- |
| 协作规则 | `check-collaboration.py`、`tests/test_collaboration.py`；供 Git hook 与协作 CI 使用 |
| 当前 HTTP／协议契约 | `check-*-contract.py`；对应 `.github/workflows` 中的产品契约检查 |
| CI 验证范围 | `ci/gate_scope.py`；供 Android 候选与授权边界 CI 使用 |
| 开发服务恢复 | [development_runtime](development_runtime/README.md)；只恢复已登记的开发实例 |
| Android 候选、隔离服务与恢复 | `prerelease/`、`android_integration/`；构建与服务／SQL 验证分别记录 |
| 评论快照真实进程与性能 | [comment-snapshot-process](comment-snapshot-process/README.md)；现行总入口为 `campaign.py`，共享 `run.py` 等模块；定向性能工具仍保留 |
| Feed 故障回归 | `feed_staging/`；TLS、故障注入及 worker 失败行为 |
| 图片联合与原生验收 | [post-media-joint](post-media-joint/README.md)、[post-media-android](post-media-android/README.md)；按本轮专属环境准备 |
| 社区设备与原生边界 | [stage1](community-android-stage1/README.md)、[stage2](community-android-stage2/README.md)、[stage3](community-android-stage3/README.md)；旧窗口已结束，复验前须重新核对任务与资源 |
| 认证设备／签名／生命周期 | `prepare/build/operate-android-b3-b4-fresh.py`、`run-android-*.py/ps1`、`control-android-*.py` 及相应 `tests/`；保留受控 runner 与安全回归，不将旧设备结果视为当前通过 |
| 原生诊断与关联文件 | [Passkey 诊断](android-passkey-diagnostic/README.md)、[TalkBack 诊断](android-talkback-diagnostic/README.md)、`configure-passkey-associations.py`；诊断不证明产品验收 |
| 邮件配置 | [smtp](smtp/README.md)；凭据只放本机私有配置 |
| 固定资源生成 | `design/build_startup_wordmark.py`、`post-image-media/protocol_vectors.py`；保留字标和协议样例的生成来源 |

## 已退役的阶段材料

2026-11-10 分两项任务清理：P5-D1 五个固定实验脚本、`community-android-stage3/execution-20261015/` 八个执行归档脚本、两个图片旧交付脚本及 F3 wrapper 共 16 项，见[已完成的 runner 清理](../docs/tasks/TEST-RUNNER-CLEANUP-20261110.md)。本轮再补清初次 OpenAPI 生成器和八个旧设备／机器独立辅助脚本，共 9 项，见[任务记录](../docs/tasks/TOOLS-CLEANUP-20261110.md)。执行归档的历史入口 README 继续保留。

这些文件从[清理前固定提交 fbf5e6b](https://github.com/zewbby/SpeechFUL/tree/fbf5e6be302432db8a187a6293403b88be454bc7/tools)追溯，不另建可执行副本。旧报告的结果、JSON 摘要、源码 SHA 和协议向量保留原值；历史命令只解释当时的执行过程。
