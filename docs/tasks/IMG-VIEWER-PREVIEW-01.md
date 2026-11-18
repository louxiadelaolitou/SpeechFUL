# IMG-VIEWER-PREVIEW-01 — 点击图片复用已加载预览

- 模式：常规
- 负责人／状态维护人：当前 Session
- 执行 Session（指挥模式）：无
- 分支／Worktree／PR：`feat/image-viewer-preview` @ `E:\speechful`（已推送 `develop`）；基线 `develop/main@315f151`；实现提交 `d4c8be4`；修正提交 `7ed7419`；文档提交 `e6b68c1`、`7f7414a`、`a761106`（状态同步见后续提交）；PR [#18](https://github.com/zewbby/SpeechFUL-Develop/pull/18)（base `main`）
- 本轮规则版本（指挥模式）：不适用
- 授权依据与边界：用户 2026-11-17 指令“现在有一个问题，我们的页面图片明明已经加载出来了，但是直接点击图片却又要再加载一遍，你看看问题出在哪里……你看看能不能优化成这样”，并批准本任务实施计划；用户 2026-11-18 指令“提pr”授权推送分支与创建 PR #18，指令“修正”授权修复审查发现的预览失败回退、私信测试、格式与记录问题。授权范围不含合并、发布、部署或版本号变更。用户附带的 X 视频当前模型无法解码查看，本次按文字诉求与现有代码实现，不冒充已查看视频。
- 需求确认：已确认目标是点击已显示的帖子／评论图片进入查看器时立即显示同一张已加载图片，原图继续加载并在首帧校验后替换；保存仍使用完整原图，评论图片不得重复请求 `/original`。未决：视频中的共享元素缩放转场是否后续追加；本次不实现，先解决重新加载。
- 更新日期：2026-11-18
- 状态：实现 完成（`d4c8be4`、`7ed7419`）；验证 通过（本次范围；真机帖子／评论／保存 PASS，私信与账号切换 NOT_RUN；修正项以自动化复验）；产品 CI PASS（代码 head `7ed7419`：25 success／1 skip，0 failure；见下表）；合并 未合并（独立审查 0，待仓库成员审查）；发布 未发布

## 目标与范围

点击 Feed、帖子详情、个人主页、转发、评论和私信里已经显示的图片时，查看器第一帧直接显示调用方已经加载并绘制的预览，不再出现“正在加载图片”或转圈；完整原图继续按原有授权路径加载，真实首帧尺寸匹配后才进入 `ready`，保存、资源上限、访问撤销、失败重试语义不变。

包含：

- 图片查看器包新增可选的、调用方所有的纯视觉 `previewProvider`；仅当前页显示，不进入来源缓存、资源计量、ready 判定或保存判定，查看器不驱逐调用方 provider。
- 帖子复用 `_PostThumbnail` 已绘制的缩略图 provider。
- 评论复用已显示的缩略图，并记忆化 `provider()`，消除缩略图与查看器各请求一次 `/original` 的重复下载；保存仍单独授权读取原图。
- 私信复用 `_photoProviders` 中已缓存的 `MemoryImage`。
- 查看器包、移动端和示例测试；README 与设计文档同步。

不包含：

- 服务端、OpenAPI、迁移、评论缩略图端点或新 header。
- Hero／共享元素缩放转场。
- 发帖编辑器本地图片查看器（provider 位于子 State，需单独提升所有权）。
- 把缩略图当作完整原图或保存源、长期磁盘缓存、版本号变更、合并、发布或部署（推送分支与创建 PR 已由用户 2026-11-18“提pr”授权）。

## 接口与依赖

- 新增 `ViewerItem.previewProvider` 与 `ViewerItem.withPreview(...)`；为向后兼容的可选参数。
- `PostImage` 新增可选 `loadedPreview` 查询函数；`PostImage.thumbnail` 作为静态兜底。
- 无服务端依赖；不修改 `packages/openapi`、迁移或启动检查。
- 依赖现有 `PostImagesSection`、`_MediaLease`、`_PostThumbnail` 与查看器生命周期。

## 验收标准

- 已显示的帖子、评论、私信图片被点击后，查看器首帧显示该图片，不再出现加载文案或转圈；完整原图随后替换，最终清晰度与原图一致。
- 帖子打开查看器不新增缩略图请求；评论缩略图与查看器合计只请求一次 `/original`；私信不重复读取照片字节。
- `status` 仍只在完整原图真实首帧尺寸匹配后变为 `ready`；保存按钮在此之前不可用，保存仍取得完整原图。
- 原图加载失败、尺寸不匹配、资源超限仍显示原有脱敏错误与重试；预览不会掩盖失败。
- 关闭、切换账号、访问撤销后预览立即消失；查看器不驱逐调用方预览 provider，不提前解码邻居页预览。

## 当前进度与证据

环境：Windows 11 25H2 x64；Flutter 3.47.5 / Dart 3.13.4（`D:\dev\flutter`）；Android SDK `D:\dev\Android\Sdk`（仅在构建命令中通过 `ANDROID_HOME`／`ANDROID_SDK_ROOT` 提供）。基线 `315f1513f9362013956676234cd6ef40c1d234b5`，分支 `feat/image-viewer-preview`，工作区 `E:\speechful`。以下自动化结果针对本任务实现提交 `d4c8be4` 与修正提交 `7ed7419`（预览失败回退、私信 UI 测试、格式统一）；真机结果针对由 `d4c8be4` 源码构建的 debug APK，修正项未重新装机，私信真机路径仍为 `NOT_RUN`。

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | `d4c8be4`、`7ed7419`（基线 `315f151`） | PASS：查看器新增 `ViewerItem.previewProvider`／`withPreview`；仅当前页绘制调用方预览，完整来源匹配首帧后替换；预览不参与 `ready`、保存、资源上限和驱逐。帖子记录实际绘制的 `ResizeImage`，评论记忆化 provider 并复用已绘制缩略图，私信复用缓存 `MemoryImage`。修正提交在预览解码失败时回退为加载指示器（保留读屏标签），避免完整来源仍在加载时留下空白画面。 |
| 查看器包静态分析 | `packages/image_viewer`，Flutter 3.47.5 | PASS：`flutter analyze --no-pub` → `No issues found! (ran in 10.2s)`。 |
| 查看器包测试 | 同上 | PASS：`flutter test --no-pub` → `00:19 +148: All tests passed!`。新增 5 项：预览先行并由完整帧替换；预览解码失败回退加载指示器且完整来源仍可进入 `ready`；尺寸不匹配不采用预览；访问失效移除预览且不驱逐调用方 provider；未选中的邻页不预解码预览。 |
| 示例静态分析 | `packages/image_viewer/example` | PASS：`flutter analyze --no-pub` → `No issues found! (ran in 4.7s)`。 |
| 示例测试 | 同上 | 部分失败：6 通过、2 失败。新增“延迟来源时预览可见、完整帧到达后替换”通过。失败项为 `iOS example stays gated independently of Android`（断言 iOS 保存按钮禁用，但 `HnuImageViewer.enableIosSaving` 默认已为 `true`）与 `example source failure retries and invalidation returns to owner`（`pumpAndSettle` 超时）；两者在基线查看器库上同样复现，见下。 |
| 示例构建 | 同上，Android SDK 路径已提供 | PASS：`flutter build bundle --debug --no-pub` 退出码 0，生成 `build/flutter_assets/kernel_blob.bin`。 |
| 移动端静态分析 | `apps/mobile` | PASS：`flutter analyze --no-pub` → `No issues found! (ran in 11.7s)`。 |
| 移动端全量测试 | 同上 | PASS：`flutter test --no-pub --concurrency=2` → `03:54 +1354 ~4: All tests passed!`，退出码 0。新增帖子预览测试断言缩略图已绘制后打开查看器立即显示预览且完整来源仅再读一次；评论测试断言缩略图与查看器合计只读一次 `/original`，拒绝后重试每次仅一次读取；私信测试断言点击已缓存照片打开的查看器复用同一 `ImageProvider`。 |
| 移动端构建 | 同上，Android SDK 路径已提供 | PASS：`flutter build bundle --debug --no-pub` 退出码 0，生成 `apps/mobile/build/flutter_assets/kernel_blob.bin`（约 70.4 MB）。 |
| 差异检查 | 同上 | PASS：`git diff --check` 无输出；`pubspec.lock` 已恢复，工作区仅含本任务实现、测试和文档。 |
| 基线失败对照 | 临时收起 `packages/image_viewer/lib` 与 `test` 后运行示例单项 | 两个失败均在基线代码复现，确认非本任务回归；随后 `git stash pop` 恢复全部实现。 |
| 真机环境与装配 | Xiaomi `24129PN74C`，Android 17／API 37，arm64-v8a；`org.hnuhole.hnuhole_mobile` 1.2.0（`2026111701`）debug APK，安装时间 2026-11-17 22:27；WSL2 Ubuntu-24.04 一次性开发 C 服务 | PASS：APK 由 `d4c8be4` 工作区构建（另含已撤销的临时 `FEED_LOAD_FAILURE` 诊断输出，预览代码未改）；`adb reverse tcp:8443`／`tcp:8444` 可用，C `/health/ready` 返回 `ready`，迁移账本 51。开发库落后于当前源码，重建 C 并以一次性开发库管理员执行 0040–0051 迁移及 `grant-community.sql`、`grant-post-images.sql`、`grant-comments.sql`、`grant-direct-messages.sql`、`grant-moderation.sql` 后服务才就绪；这是本机开发环境修复，不是产品改动。 |
| 真机帖子预览 | 同上；在应用内选择 2400×1600 JPEG（256,652 字节）发布“安利”帖 | PASS：暂停 C 进程（`SIGSTOP`）后点击信息流中已显示的帖子图片；打开查看器约 0／0.5／1.5 秒的截图均直接显示该图片，无转圈、无“正在加载图片”，保存按钮为禁用态（`图片加载完成后可保存`）。本地未提交截图：`.zcode/viewer-slow-t0.png`、`.zcode/viewer-slow-t500.png`、`.zcode/viewer-slow-t1500.png`。 |
| 真机失败与重试 | 同上；保持 C 暂停至客户端超时 | PASS：超时后显示 `图片加载失败`／`重试加载`，预览不掩盖失败；恢复 C（`SIGCONT`）并点“重试加载”后进入完整图片，保存按钮变为 `保存图片`（`.zcode/viewer-ready.png`）。 |
| 真机评论预览与单次读取 | 同上；在帖子详情为同一图片发布评论 | PASS：评论缩略图显示后暂停 C，再点击评论图片；查看器立即进入可保存的完整图片状态，未发出新的网络请求也未被暂停影响，证明缩略图与查看器共用已记忆化的 provider（`.zcode/comment-viewer-suspended.png`）。 |
| 真机保存原图 | 同上；查看器内点击保存 | PASS（范围受限）：提示 `已保存到相册`，`/sdcard/Pictures/original (1).jpg` 为 256,652 字节，与发布原图字节数一致；未做 SHA-256 比对，不能据此断言二进制完全一致。保存路径复用原图 resolver，未使用缩略图或预览代替保存源。 |
| 真机访问撤销／账号切换／私信照片 | 同上 | NOT_RUN：未取得可切换账号的密码，避免损失当前会话；本机没有含照片的私信会话，未发送首条图片消息。失败重试已覆盖，访问撤销与私信照片仍待有会话条件时补验；个人主页／转发复用同一查看器组件路径，未逐页重复。 |
| 私信专项测试 | `apps/mobile/test/dm_chat_test.dart` | PASS：`flutter test --no-pub --timeout 120s test/dm_chat_test.dart` → `00:05 +10: All tests passed!`。新增用例保存并同步一张本地加密照片，在 `DmChatRoute` 渲染后点击图片，断言 `HnuImageViewer` 的 `ViewerItem.previewProvider` 与页面已绘制的 `Image.image` 为同一 provider；关闭查看器、撤下页面后在假时钟下排空存储队列并 `detach`，加密数据库正常关闭。 |
| 产品 CI／PR | PR [#18](https://github.com/zewbby/SpeechFUL-Develop/pull/18)，head `7ed7419` | PASS：26 项检查 25 success、1 skip（`actual-process-acceptance` 按设计跳过），无 failure；`compile`、`flutter`、`flutter-and-android`、`flutter-and-real-feed`、`go-postgres` 等通过。首轮 `rehearsal` 因下载 `golang.org/x/text@v0.21.0` 时 `proxy.golang.org` 流中断失败（与本改动无关），重跑该作业后 success。协作 CI 不代表真机或生产验收。 |
| 独立审查 | PR [#18](https://github.com/zewbby/SpeechFUL-Develop/pull/18) | 未满足：0 条独立 review；作者不能自批，需仓库成员审查后才能合并。 |
| 合并／发布 | — | 未合并、未发布、未部署。 |

## 阻碍与下一步

- 示例套件两项失败是基线既有问题，不阻塞本任务；若后续统一清理，需单独确认 `enableIosSaving` 产品决策并排查示例重试场景的超时，不并入本次图片预览改动。
- 真机已覆盖 Feed 图片预览、失败重试、评论复用与保存原图；私信照片、账号切换和访问撤销尚未执行，需在有含图私信会话及用户本人配合切换账号时补验。私信预览复用已补 UI 测试（provider 同一性），真机仍 `NOT_RUN`。个人主页／转发复用同一 `PostImage`／查看器路径，未逐页重复。
- 视频中的共享元素缩放转场仍未实现；如需要，另行设计路由、受保护媒体与手势验收。
- PR #18 已送审；独立 review 为 0，作者不能自批，合并前需仓库成员审查。合并、正式部署仍需明确授权，本次不推进。
