# Android 预发布与隔离演练

本页是当前工具入口。2026-10-18 至 10-20 的候选、设备和故障回执已从当前目录退役；原始文件保存在[整理前的固定提交](https://github.com/zewbby/SpeechFUL-Develop/tree/f5127dc9566daf545afe9c2ce26cd233a4bc54dd/docs/release)。当前 Android 发布与验收状态见[发布任务](../tasks/ANDROID-BETA-LAUNCH-20261109.md)，不能从历史候选推断现版本通过。

## CI 候选

[Android prerelease workflow](../../.github/workflows/android-prerelease.yml) 在受影响的 PR 上运行，也接受手动指定完整 `source_sha` 和正整数 `build_number`。它固定检出源码提交、恢复 Git LFS、构建并测试签名 ARM64 APK，上传 APK、manifest 和校验和。构建成功只证明该提交的候选产物；默认 API 地址为 `.invalid`，测试签名不用于稳定升级或正式发布。运行前核对新仓库的 workflow、依赖与所选 SHA。

## 本地隔离演练

[environment.py](../../tools/prerelease/environment.py) 只用于明确归属的一次性 Linux 开发环境。先在 WSL2 Linux 文件系统中准备全新绝对路径、干净的已提交源码、可用的 Go、Goose、PostgreSQL、Mailpit 和 Python/Pillow；工具不会自动下载依赖。初始化需要一条确实可回滚的完整提交 SHA。

```sh
python tools/prerelease/environment.py --root /absolute/owned/lab init --rollback-sha FULL_MAIN_SHA
python tools/prerelease/environment.py --root /absolute/owned/lab rehearse
```

`init` 创建独立 C/V 数据库、私有材料和测试资源；`rehearse` 执行候选的隔离链路并生成 `evidence.json`。只将脱敏的 `evidence.json` 用作公开证据；同目录的数据库、密钥、邮件和日志保持私有。

如需在同一个已初始化的 root 内手动检查，使用 `start`、`health`、`stop`；恢复冻结 Gate 只通过明确的 `recover` 命令并按运行授权执行。停止不等于删除数据或清理其他任务资源。命令的实际边界以脚本 `--help`、[API 说明](../../services/api/README.md)和[基础设施说明](../../infra/README.md)为准。

此链路不代替正式签名、真实用户数据升级、设备验收或生产发布。
