# 2026-10-15 一次性执行源码的历史入口

本次实验的 8 个执行脚本已从当前树退役，原始字节保留在
[固定提交的执行归档](https://github.com/zewbby/SpeechFUL/tree/fbf5e6be302432db8a187a6293403b88be454bc7/tools/community-android-stage3/execution-20261015)。
其中包括固定当次环境的 `lab.py`、两个 SQL／代理观察脚本、两个需复制进镜像才能运行的 Dart 夹具，以及三个 shell wrapper。它们没有进入当前正常功能测试的发现路径或 CI。

历史摘要仍见[机器记录](../../../docs/design/community-android-stage3-device-validation-20261015.json)的 `executedSourceArchive`；[实测报告](../../../docs/design/community-android-stage3-device-validation-20261015.md)中的结果与边界保持原意，不因退役而重写。历史脚本绑定旧路径、owner、源码和一次性 started 标记，不适用于当前环境，不能直接重放。

当前认证功能测试、真实 SQL／设备 runner、Stage 3 准备工具及守卫测试继续保留。删除依据与本轮验证见[清理任务](../../../docs/tasks/TEST-RUNNER-CLEANUP-20261110.md)。此次未操作旧实验环境、数据库或设备。
