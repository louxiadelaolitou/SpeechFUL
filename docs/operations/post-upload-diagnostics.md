# 图片发布排障

## 在哪里看

Android 安装包含本功能的新构建后，在 IDEA 的 Terminal（PowerShell）运行：

```powershell
E:\Hnuhole-env\android-sdk\platform-tools\adb.exe logcat -v time -s flutter | Select-String 'POST_UPLOAD'
```

其他电脑用其 SDK 的 adb 路径；多台设备时加 `-s <设备序列号>`。手机须允许 USB 调试。IDEA 的 Run 控制台只显示由该运行配置启动的进程。旧版 APK 不会因为源码更新就自动产生新日志。

输出包含本机请求序号 `trace`、操作 `operation`、事件 `event`、阶段 `stage`、累计耗时 `elapsed_ms`、`http_status`、`request_id`，完成事件还包含 `outcome`。这些是手机本地日志，不发往新日志服务，不记录完整 URL、任务/账号 ID、令牌、正文、图片内容或响应正文。

- `submit_images`：受理带图发布／原任务重试命令。
- `check_images`：查询原图片是否已到服务端。
- `upload_grant`：申请该图片的上传许可。
- `upload_bytes`：上传原图。
- `connect`：连接准备；`send_body_and_wait_response`：发送请求并等待响应头；`read_response`：读取响应体；`validate_response`：校验响应。
- `success`：协议校验成功；`timeout`：超过现有请求期限；`network`：网络 I/O 错误；`invalid_response`：收到不符合协议的响应；其他值为既有 PostFailure 分类。

发送原图和等待响应头共用一个阶段，不能仅凭它判断上行带宽或服务端处理耗时。没有收到响应头时，HTTP 状态和请求 ID 为 null；响应头缺少有效的服务端请求 ID 时同样为 null，不伪造它。trace 仅用于同一 App 进程内归组，不通过请求头传给 C/V。

例如 `operation=upload_bytes`、`stage=send_body_and_wait_response`、`outcome=timeout`、`http_status=null` 表示原图上传未在期限内获得响应；它本身不能证明服务端拒绝或帖子未受理。按照原任务核对，避免另发重复帖子。

## 任务状态与后端日志

任务详情页可显示图片未确认上传、待补传序号及“继续上传”。数据库中的 ACCEPTED 仅表示帖子任务受理，图片未齐时不能视为发布成功。当前个人列表仍可能统一显示“发布中”；本改动不调整产品状态展示。

后端进程由谁启动，就去对应进程的输出或配置日志文件查看。当前公网试用环境的社区日志由部署所有者定位在 WSL 中；机器路径不是通用部署规范。已有日志有 worker 汇总，并未覆盖所有上传 HTTP 请求，不能保证用请求 ID 能在后端找到对应记录。任务库只读诊断需要受控权限，不把数据库密码放进命令、日志或 Git。

## 上传专用代理采集

现有 Nginx 部署可用 `tools/post_upload_diagnostics/capture_proxy.py` 进行短时诊断。工具必须以部署 owner 运行，传入当前配置、实际 nginx 可执行文件、新的私有输出目录、事先核对的配置 SHA256 和采集秒数（30–600）。它先验证候选配置，仅对社区原图 PUT 输出日志，平滑 reload；正常结束或 Ctrl+C 会恢复原配置。配置并发变化时停止覆盖并保留备份，需人工核对。不要在未核对配置和部署归属时运行，也不要同时开启多个采集进程。

日志固定 route 标签，不含完整 URI、正文、凭据或身份 ID。`request_length` 包含请求头，不能直接等同原图字节数；`declared_body_length` 为筛选后的 Content-Length。完成日志在请求结束后出现，手机超时后代理仍可能继续收体，不能立刻把空日志认定为请求没到达。

结合长度、完成标志及 upstream 字段判断：请求总长度还小于原图声明长度，且所有 upstream 信息为空，说明该次请求体未收齐、尚未转交 Go。upstream 已出现时再按需要观察后端收体／校验／事务分段；`upstream_header_time` 不是单独的图片解码耗时。不记录到请求 ID 时保持空值，不绕过认证重新发请求来获取它。

此工具不调整上传超时或缓冲策略。采集后核对配置摘要精确恢复及服务健康，原始配置备份和日志仅保存在部署私有目录，不提交 Git。

## 现有运维能力的边界（汇总）

仓库已有[运行入口](README.md)、健康检查、后台任务汇总日志、部分模块指标以及[日常环境恢复器](../../tools/development_runtime/README.md)。这些分散能力不等于统一运维平台。当前没有覆盖全部业务的请求链路日志、统一发帖任务查询界面或本次核实可用的统一日志检索／告警入口。本文件只交付图片上传的手机日志查看方法；不宣称已完成全站监控和告警。
