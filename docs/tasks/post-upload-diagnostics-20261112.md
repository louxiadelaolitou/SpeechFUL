# post-upload-diagnostics-20261112 — 图片上传诊断日志

- 模式：常规；负责人：当前 Session。
- 分支／Worktree：`codex/fix/post-upload-diagnostics`；`E:/Hnuhole-env/source/SpeechFUL-post-upload-diagnostics`；基线 `9937638c077bf2da7d51daa5de13a83645dc4320`。
- 授权依据：用户反馈带图帖长时间发布中，允许连接手机调查；明确同意补齐阶段、耗时、HTTP 状态及请求 ID。仅诊断能力，不自动重发真实帖子，不改数据库或正式部署，不沿用其他任务的 Push/合并/Release 授权。
- 需求确认：现有发布行为保持；Android 本地输出可在 IDEA Terminal 用 adb 查看。缺少响应的状态码和请求 ID 留空，不猜测；日志不含敏感正文、凭据和资源标识。无未决产品取舍。
- 更新日期：2026-11-12。
- 状态：实现完成；验证本地 PASS，Android 构建待验；未合并；未发布。

## 目标与范围

为带图受理、图片状态查询、上传许可和原图 PUT 增加设备本地分阶段日志，及实际可操作的查看文档。记录本次排障局限：列表“发布中”没有体现详情里的图片待补传；具体图片失败原因无法从旧版日志追溯。日志改动不冒充原问题修复或完整运维平台。

## 验收标准

- 实际 HTTPS 成功、503、无效响应和超时都有耗时、阶段和结果，收到响应时保留状态及安全请求 ID。
- 不自动重试，不改变认证、幂等键和发帖状态；不泄露请求／响应正文、身份或上传令牌。
- 提供 IDEA Terminal 的 adb 查看方法，明确旧 APK 无新日志。

## 当前进度与证据

- PASS：首轮 post_api_test.dart 41 项；包含新 HTTPS 诊断用例。静态分析无问题。
- PASS：最终静态分析零问题；`flutter test --no-pub test/post_api_test.dart test/post_images_test.dart` 共 64 项通过，包含全部错误场景日志的脱敏断言。Windows / 锁定离线依赖；证据 `E:/Hnuhole-env/post-publish-diagnosis-20261112/focused-tests.log`。
- 被测版本：基线 9937638 + 本提交的上传日志与测试差异；Android Release 构建另行记录，不宣称已完成原帖真实上传验收。
- 当前用户帖子仅做了只读任务查询和查看页面，没有补传／取消／删除；实际证据位于任务私有目录，不提交 Git。

## 下一步

完成最终聚焦验证及 Android 构建，交付诊断版本。Push、PR、合并、正式发布未执行。
