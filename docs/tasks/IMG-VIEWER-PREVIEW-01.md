# IMG-VIEWER-PREVIEW-01 — 点击图片复用已加载预览

- 模式：常规
- 负责人／状态维护人：当前 Session
- 执行 Session（指挥模式）：无
- 分支／Worktree／PR：`feat/image-viewer-preview` @ `E:\speechful`；基线 `develop/main@315f151`；PR 未创建
- 本轮规则版本（指挥模式）：不适用
- 授权依据与边界：用户 2026-11-17 指令“现在有一个问题，我们的页面图片明明已经加载出来了，但是直接点击图片却又要再加载一遍，你看看问题出在哪里……你看看能不能优化成这样”，并批准本任务实施计划。授权范围为代码、测试、文档与必要的本地提交；未授权推送、创建 PR、合并、发布或部署。用户附带的 X 视频当前模型无法解码查看，本次按文字诉求与现有代码实现，不冒充已查看视频。
- 需求确认：已确认目标是点击已显示的帖子／评论图片进入查看器时立即显示同一张已加载图片，原图继续加载并在首帧校验后替换；保存仍使用完整原图，评论图片不得重复请求 `/original`。未决：视频中的共享元素缩放转场是否后续追加；本次不实现，先解决重新加载。
- 更新日期：2026-11-17
- 状态：实现 完成；验证 通过（本次范围；示例 2 项基线既有失败，真机 NOT_RUN）；合并 未合并；发布 未发布

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
- 把缩略图当作完整原图或保存源、长期磁盘缓存、版本号变更、推送、PR、合并、发布或部署。

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

环境：Windows 11 25H2 x64；Flutter 3.47.5 / Dart 3.13.4（`D:\dev\flutter`）；Android SDK `D:\dev\Android\Sdk`（仅在构建命令中通过 `ANDROID_HOME`／`ANDROID_SDK_ROOT` 提供）。基线 `315f1513f9362013956676234cd6ef40c1d234b5`，分支 `feat/image-viewer-preview`，工作区 `E:\speechful`。以下结果均针对本工作区未提交差异。

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | 基线 `315f151` + 本任务差异 | PASS：查看器新增 `ViewerItem.previewProvider`／`withPreview`；仅当前页绘制调用方预览，完整来源匹配首帧后替换；预览不参与 `ready`、保存、资源上限和驱逐。帖子记录实际绘制的 `ResizeImage`，评论记忆化 provider 并复用已绘制缩略图，私信复用缓存 `MemoryImage`。 |
| 查看器包静态分析 | `packages/image_viewer`，Flutter 3.47.5 | PASS：`flutter analyze --no-pub` → `No issues found! (ran in 10.7s)`。 |
| 查看器包测试 | 同上 | PASS：`flutter test --no-pub` → `00:17 +147: All tests passed!`。新增 4 项：预览先行并由完整帧替换；尺寸不匹配不采用预览；访问失效移除预览且不驱逐调用方 provider；未选中的邻页不预解码预览。 |
| 示例静态分析 | `packages/image_viewer/example` | PASS：`flutter analyze --no-pub` → `No issues found! (ran in 4.7s)`。 |
| 示例测试 | 同上 | 部分失败：6 通过、2 失败。新增“延迟来源时预览可见、完整帧到达后替换”通过。失败项为 `iOS example stays gated independently of Android`（断言 iOS 保存按钮禁用，但 `HnuImageViewer.enableIosSaving` 默认已为 `true`）与 `example source failure retries and invalidation returns to owner`（`pumpAndSettle` 超时）；两者在基线查看器库上同样复现，见下。 |
| 示例构建 | 同上，Android SDK 路径已提供 | PASS：`flutter build bundle --debug --no-pub` 退出码 0，生成 `build/flutter_assets/kernel_blob.bin`。 |
| 移动端静态分析 | `apps/mobile` | PASS：`flutter analyze --no-pub` → `No issues found! (ran in 8.0s)`。 |
| 移动端全量测试 | 同上 | PASS：`flutter test --no-pub --concurrency=2` → `03:36 +1353 ~4: All tests passed!`，退出码 0。新增帖子预览测试断言缩略图已绘制后打开查看器立即显示预览且完整来源仅再读一次；评论测试断言缩略图与查看器合计只读一次 `/original`，拒绝后重试每次仅一次读取。 |
| 移动端构建 | 同上，Android SDK 路径已提供 | PASS：`flutter build bundle --debug --no-pub` 退出码 0，生成 `apps/mobile/build/flutter_assets/kernel_blob.bin`（约 70.4 MB）。 |
| 差异检查 | 同上 | PASS：`git diff --check` 无输出；`pubspec.lock` 已恢复，工作区仅含本任务实现、测试和文档。 |
| 基线失败对照 | 临时收起 `packages/image_viewer/lib` 与 `test` 后运行示例单项 | 两个失败均在基线代码复现，确认非本任务回归；随后 `git stash pop` 恢复全部实现。 |
| 真机／设备观察 | — | NOT_RUN：本环境无已连接设备；未以单元测试或 bundle 构建代证真机首帧观感、慢网替换、保存原图与账号切换行为。 |
| 私信专项测试 | — | NOT_RUN：`DmChatRoute` 需要完整运行时／身份装配，本次仅有一行 `previewProvider` 接入，未新增 DM UI 测试；已由静态分析覆盖编译与类型。 |
| 产品 CI／PR | — | NOT_RUN：未推送、未创建 PR。 |
| 合并／发布 | — | 未合并、未发布、未部署。 |

## 阻碍与下一步

- 示例套件两项失败是基线既有问题，不阻塞本任务；若后续统一清理，需单独确认 `enableIosSaving` 产品决策并排查示例重试场景的超时，不并入本次图片预览改动。
- 真机验证仍未执行；需要在可连接 Android 设备上按已确认场景检查 Feed、详情、个人主页、转发、评论、私信，并覆盖慢网、失败重试、保存原图与账号切换。
- 视频中的共享元素缩放转场仍未实现；如需要，另行设计路由、受保护媒体与手势验收。
