# post-upload-diagnostics-20261112 — 图片上传诊断日志

- 模式：常规；负责人：当前 Session。
- 分支／Worktree：`codex/fix/post-upload-diagnostics`；`E:/Hnuhole-env/source/SpeechFUL-post-upload-diagnostics`；基线 `9937638c077bf2da7d51daa5de13a83645dc4320`。
- 授权依据：用户反馈带图帖长时间发布中，允许连接手机调查；明确同意补齐阶段、耗时、HTTP 状态及请求 ID。仅诊断能力，不自动重发真实帖子，不改数据库或正式部署，不沿用其他任务的 Push/合并/Release 授权。
- 需求确认：现有发布行为保持；Android 本地输出可在 IDEA Terminal 用 adb 查看。缺少响应的状态码和请求 ID 留空，不猜测；日志不含敏感正文、凭据和资源标识。无未决产品取舍。
- 更新日期：2026-11-12。
- 状态：实现完成；验证本地、Android 构建及真机日志采集 PASS；未合并；未发布。

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

诊断版本已安装并实际采集日志；后续继续分析原图 PUT 为何未在既有期限内取得响应。没有调整超时、重试或列表状态行为，不宣称原问题修复。Push、PR、合并、正式发布未执行。

## Android 诊断构建

- PASS：实现提交 `997c0e2ef35fa381b1e9e7f54cc53bc2905f319b`，锁定依赖，`flutter build apk --release --no-pub --target-platform android-arm64`；包名仅在隔离打包副本保留 publictrial，与源码导出比对一致。构建命令与结果位于任务私有目录的 build-diagnostic.ps1 / release-build.log / diagnostic-source-proof.json。
- PASS：`1.1.1-diagnostic`（2026111202），原签名，APK SHA256 `18a5c448e19f863b10750278cdaa4894b3999eeddccfac8f5b598f99406ae7f8`；覆盖 vivo V2323A 后启动，首次安装时间保持 `2026-11-10 12:32:23`。用户在手机确认安装并回复“好了”。GitHub Release 附件未变。
- PASS：限时只采集 POST_UPLOAD；实际手机 Release 日志可见。用户说明原任务已取消回草稿，只读确认 CANCELLED；未替用户恢复、取消或提交。采集到的是用户后续的新发布，不归作原任务补传。
- 第一组真实日志（11:15）：提交返回 202，图片状态和许可返回 200；原图在 send_body_and_wait_response 阶段 15003 ms 超时，http_status/request_id 为 null。只读确认约 5.7 MB 图片未到存储，该任务随后由用户取消。证据 phone-upload-live.log。
- 用户要求采集开启后再次点击发布；第二组完整日志（11:18）：提交返回 202（1537 ms），图片状态返回 200（1688 ms），许可返回 200（1346 ms）；原图 PUT 再次在 15003 ms 超时，未收到响应头。证据 phone-upload-second.log，trace 9–12；文件中更早缓冲日志不属于第二组。
- 结论：原图上传未在既有 15 秒总期限内取得响应；尚不能区分上行、代理或后端耗时。诊断能力验收不等于原发布问题修复。

## GPT-6 Astra High 独立只读诊断

用户明确要求使用 GPT-6 Astra、High 推理；该诊断以 1126c47 为基线，只读核对代码和已有真机证据，无实现、部署或发帖操作。

- 确认原图 PUT 的 15 秒总期限覆盖连接、发体、等待及读取响应；trace 12 连接已耗 1936 ms，后续约剩 13 秒。5.7 MB 和 2304×4096 低于源码大小／像素上限，但不代表解码验证已通过。
- 确认上传失败后 mediaUploadPaused 会阻止自动补传，个人列表却只按 ACCEPTED 绘制 pending 转圈，这是独立的状态展示缺陷。
- 不能确认慢段：Nginx 请求体缓冲警告不证明收齐或失败；源码图片校验槽位等待发生在 8 秒解码期限之前。运行后端二进制缺可核对 Git SHA，不把源码机制冒充运行版本实证。
- 建议先仅增加上传代理的脱敏长度、完成标志、upstream 状态和分段耗时，判断客户端到期时是否进入 upstream；再定位必要的后端收体／校验排队／事务阶段。原图 PUT 独立预算应由测量确定，普通 API 和连接上限保持。暂停状态展示属于后续待确认产品变更。
- 原公网大图发布成功闭环仍待验；既有 64 项测试和 Release 构建不替代该项。独立 AI 诊断不是 GitHub 成员批准。
