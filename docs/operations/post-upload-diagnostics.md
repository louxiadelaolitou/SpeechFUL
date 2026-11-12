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

## 现有运维能力的边界

仓库已有[运行入口](README.md)、健康检查、后台任务汇总日志、部分模块指标以及[日常环境恢复器](../../tools/development_runtime/README.md)。这些分散能力不等于统一运维平台。当前没有覆盖全部业务的请求链路日志、统一发帖任务查询界面或本次核实可用的统一日志检索／告警入口。本文件只交付图片上传的手机日志查看方法；不宣称已完成全站监控和告警。
