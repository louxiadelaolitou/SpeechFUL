# PROFILE-CARD-FIXES-01 — 我的页卡片与头像裁切四修

- 模式：常规
- 负责人／状态维护人：当前 Session
- 分支／Worktree／PR：`feat/profile-improvement`（仓库工作区）；实现基线 `213b8b0`；rebase 后基线 `dbbc249`（`origin/main`，含 #21/#22/#23）；承载提交为本分支 HEAD（即 PR head；amend 只改写该提交，不改变承载关系）；无 PR
- 授权依据与边界：用户 2026-11-30 消息要求修复我的页发现的四个问题并逐项确认取舍；仅前端渲染、交互入口与裁切参数，不改后端接口、权限边界与正式数据
- 需求确认：用户确认①头像裁切为微信式，最小缩放时裁切圆最大内切于图片（圆边与图片边缘相切，可继续缩小不在本次范围）②收藏/爱心只去掉“发送中”误报，真实未确认仍提示；三点菜单与帖子详情一致，不同页签不同处理，转帖页签保留专用菜单 ③图片预览组件只有一个，首页与我的页统一按新方案 ④间距按实现方提出的数值；比例阈值 1:2／2:1 可用；取消“点击查看完整图片”提示；回复页签不动；转帖页签菜单保留
- 更新日期：2026-11-30
- 状态：实现 已完成；验证 PASS（本地聚焦 PASS＋用户确认真机效果）；合并 未合并；发布 未发布

## 目标与范围

1. 头像裁切：`InteractiveViewer` 缩放下限改为按图片长短边计算，最小缩放时裁切圆最大内切于图片（图片短边等于圆的直径）。`identity_avatar_flow.dart`。
2. 我的页卡片交互：收藏／爱心点击期间的“结果尚未确认，请重试核对。”误报移除（`notice` 只读 `writeError`）；三点改为竖排并与帖子详情共用同一菜单实现（`PostActionMenu`，分享／编辑／举报／删除，权限沿用详情的能力查询），热区 30→40dp。
3. 帖子图片（首页与我的页共用的 `feedPreview` 路径）：单图宽度铺满、高度按原比例不裁切不留边，窄于 1:2 的超高图取中段方形；多图容器高度固定、每张高度等于容器高、宽度按原比例，超出 1:2／2:1 的改为中心方形；删除旧的宽度夹取、`BoxFit.contain` 与“点击查看完整图片”提示。
4. 我的页卡片间距收紧：内边距 15/13→12/10，作者行→标题 12→8，标题→正文 7→4，正文→图片 18→8，操作行下边距 10→6。

### 第二轮（用户 2026-11-30 追加，已确认全部取舍）

1. 我的页点赞选中色改为主题 `error`（红色实心爱心），收藏保持蓝色书签。
2. 首页自绘动作图标抽成公共 `FeedActionIcon`，首页与我的页共用；我的页评论入口打开帖子；**首页评论入口同样改为打开帖子**，首页卡片的快速回复弹层（`_reply`／编辑器缓存／`_replyOpen`）随之移除。
3. 仅多图：图片区移出卡片左右内边距，容器满屏宽，列表左内边距 20（未滑动时与标题左对齐）、右侧出血到屏幕边缘、滑动可到屏幕左缘；单图保持原样（两侧各 20）。首页 `feedPreviewInset` 为 0，不受影响。
4. 切到未访问页签的目标偏移由硬编码 212 改为运行时 hero 实际高度（实测 301），落在“头像区收起、标签栏固定、第一张卡片贴顶”；浅于 hero 高度时保持原位；已访问页签仍用记忆位置。

### 第三轮（用户 2026-11-30 追加，已确认）

1. 切页签改为 Threads 式：不再记忆各页签滚动位置、不再跳转（原 212/hero 高度目标偏移一并删除），只替换内容，整体滚动位置与组件相对屏幕位置不变；新页签内容暂时不足时，待布局完成后补回同一位置。切回原页签也不会跳回旧深度。
2. 图片查看器加入“从原图位置放大到整屏”过渡：`HnuImageViewerRoute` 新增可选 `origin`，以裁切框从缩略图矩形长到全屏 + 同步缩放实现；`post_images.dart` 的网格与横滑卡片传入被点缩略图的屏幕矩形。减少动效偏好下回退为原来的淡入。
3. 卡片互动按钮加入点击反馈：`_FlatCardAction` 点击时图标 160ms 脉冲（`HhMotion.quick`），数量改用 `HhAnimatedCount` 滚动；按压仍有 `HhPressFeedback` 缩放。

### 第四轮（用户 2026-11-30 追加，已确认）

1. 卡片操作行上方间距改为 6，与操作行到卡片底边的 6 一致。
2. 首页卡片改为复用「我的」页扁平卡片：`_ProfileFlatCard`/`_FlatCardAction` 提升为公共 `PostFlatCard`/`PostFlatAction`，首页保留标签、浏览数、作者页/私信入口、原有三点菜单（分享/转帖/举报/不感兴趣）与分享/转帖按钮，撤掉旧的 `HhXPostItem` 卡片；首页列表改为通栏（由卡片自带 20 内边距与底部分割线），时间显示改用我的页同款相对时间。
3. 图片放大过渡不再受无障碍“减少动效”屏蔽（用户明确要求忽略该设置）；无 `origin` 时仍按原规则淡入。
4. 切页签真正保持滚动位置：目标页签数据未就绪时只切高亮、内容保持不动，等数据到位后在下一帧再切换，避免加载态把滚动位置夹回顶部；不再记忆各页签旧位置。

### 第五轮（用户 2026-11-30 追加）

1. 卡片更紧凑：头像行→标题 8→4；正文→图片 8→4。
2. 卡片与帖子详情右上角操作按钮改为横向三点（`Icons.more_horiz_rounded`）；转帖/回复卡片同款。
3. 首页卡片取消圆角外框与间隙（`HhFeedPostCard(flat: true)`），沿用卡片底部分割线，与我的页一致。
4. 首页卡片不再显示浏览量（详情页保留）。
5. 卡片按钮统一顺序：点赞、收藏、评论、分享；分享只留图标（`PostFlatAction` 支持空 label 不渲染文字）。我的页卡片同步为点赞、收藏、评论（转帖按钮仍在末尾）。
6. 详情页交互栏顺序同上（点赞、收藏、评论、转帖、分享），改为 `Expanded` 等距铺满，并把交互栏移出详情 8px 内边距，铺到屏幕左右边缘。

### 第六轮（用户 2026-11-30 追加，用户复述前已按问题修复）

1. 间距再收紧：头像行→标题 4→2；正文→图片 4→2；正文行高 1.75→1.6。
2. 三点改横向：共享菜单 `PostActionMenu` 的图标此前仍用 `DetailChrome.more`（我自己上一轮替换没命中），现改 `Icons.more_horiz_rounded`；转帖“原帖已不可用”行的按钮同步横向。
3. 首页卡片间隙：旧代码每张卡后残留 `SizedBox(height: 12)`，已删除；实测相邻卡片 281→281 无间隙、列表仅两个卡片子节点。
4. 详情交互栏等距：`PostRepostButton` 在控制器不支持转帖时仍占一个 `Expanded` 空白格，造成“三左一右”；改为 `if (c.supportsReposts)` 才加入该格。

### 第七轮（用户 2026-11-30 报回归）

1. 页签切换回归根因：第四轮引入的“目标页签数据未就绪就先不换内容”会把上一页签内容残留在空页签下；数据到达后才换，届时空页签可滚动范围为 0，位置被夹到顶部，表现为“残留消失后继续滑动跳回页面顶部”。已删除该延迟机制（`_pendingTab` / `_maybeResumePendingTab`），恢复“立即换内容”；`_restoreTabScrollOffset` 改为等目标页签数据落定后一次性收口到可达位置（有内容保持原位置，空页签稳定在顶部），不再先停顶部再跳回。新增用例：深滚后切到空页签只显示空状态、位置稳定在 0、继续滑动不再跳。
2. 帖子详情交互按钮加入与卡片一致的点击反馈：`DetailAction` 改为有状态组件，点击时图标 160ms 脉冲（`HhMotion.quick`），数量继续用 `HhAnimatedCount`；减少动效偏好下不播脉冲。

### 第八轮（用户 2026-11-30）

空页签回弹修复：原来空状态用 `SliverFillRemaining(hasScrollBody: false)`，在该页的 sliver 结构下不产生可滚动长度（实测 `maxScrollExtent = 0`），所以切到空页签时位置被夹到顶部。新增 `_emptySliver`：用 `SliverLayoutBuilder` 取视口高度，占位高度 = 视口高度 − 页签栏 56，文字在空白区内居中（高度变化时位置随之调整）。替换回复／喜欢／收藏／转帖／“转帖服务暂不可用”以及帖子页签空状态的占位。实测：深滚 530 切到空页签后停在 301（= 头像区高度，页签栏固定、空白区铺满），继续下滑不再回弹；空状态文字位于空白区垂直中心。

### 第九轮（用户 2026-11-30）

底部导航按钮动画：`HhAppNavigation` 抽出 `_NavButton`（有状态），保留原有选中色／填充过渡与按压缩放，点击时再做一次与帖子卡片一致的 160ms 图标脉冲（1→1.16→1）；减少动效偏好下只变状态。钥匙 `destination:<name>`、tooltip 与选中语义不变。

### 第十轮（用户 2026-11-30）

1. 回复页签“回复服务暂不可用”：该文案来自 `_loadReplies` 取不到 `CommentScope`（预览夹具 `lib/dev/preview_support.dart` 没有装配 `CommentRuntime`，正式入口 `main.dart` 有）。原实现用一条靠左的 `SliverToBoxAdapter(TextButton)`，所以既不居中也不够高。改为和其他页签一致的 `_emptySliver`：占满页签栏以下可用高度、文字居中，并在下方给“重试”；已有内容时的追加失败仍保留原行内提示。
2. 底部导航动画按页签区分（320ms，一次点击播放）：首页上跳、通知（铃铛）左右摇摆并衰减、私信气泡弹一下、我的头像轻晃；选中色／填充过渡与按压缩放保留，减少动效偏好下不播。

### 第十一轮（用户 2026-11-30，预览可测回复页签）

`lib/dev`（本地排除目录，不进版本库、不影响正式包）：新增 `PreviewCommentHttp`（只读假 C 接口，只回答 `GET /api/v1/me/comments`，其余返回 404）与 `CommentRuntime` 装配（`NativeCommentJournal` 用内存 `_Store`），`HomeTestFixture.create` 增加 `replyRows` 参数；`preview_app.dart` 为两条自有帖子各准备一条回复。预览里「回复」页签现在显示真实结构化的回复卡片，不再是“回复服务暂不可用”；点回复可进帖子详情。详情页评论区仍不可用（预览没有完整评论区数据）。

### 第十二轮（用户 2026-11-30）

切到内容较少的页签（如只有一两条回复）仍会被夹回顶部：该页签总长度不足一屏，`maxScrollExtent` 小于“头像收起”所需偏移。新增组级尾部填充 `_tailFiller`：用 `SliverLayoutBuilder` 的 `precedingScrollExtent` 计算差值，补足到 `视口高度 + 头像区高度`；内容本来足够时不产生任何空白（实测长列表仍为原 extent）。空页签仍用 `_emptySliver`，填充自动为 0，不重复加长。实测：深滚 530 切到 1 条回复的页签停在 301 并保持；切到空页签同样停在 301；继续滑动不回弹。

不包含：回复页签图片组件、转帖页签专用菜单、服务端接口与数据。

补充（同日用户确认）：升级本地预览夹具 `apps/mobile/lib/dev/`（该目录由 `.git/info/exclude` 本地排除、不进版本库、不影响正式包），让四处改动可在无后端时自测：`PreviewPostsApi` 补上 `PostInteractionsApi`／`PostSharingApi` 与真实 `capabilities`，爱心收藏与三点菜单可实际操作；`PreviewPostMedia` 用内存绘制图代替受保护媒体接口，演示帖带 3:4／16:9／1:1／1:3 长截图／3:1 全景，覆盖可见状态。

## 接口与依赖

无新增接口。复用既有 `PostShareAction`、`_confirmScoped`、`_PublishedPostEditScreen`、`controller.postCapabilities/deletePost/openPostEdit`；插入/删除能力只在 `own` 卡片查询，避免列表页额外请求。

## 验收标准

- 头像裁切能把图片缩到圆与图片边缘相切且不能再小；拖动/缩放后确认输出正常。
- 连续点击收藏与爱心不再出现“结果尚未确认”提示；真实失败或未确认仍显示提示与“重试核对”。
- 我的页卡片三点为竖排，点击出现与帖子详情一致的菜单；转帖页签仍为转帖专用菜单；卡片其余区域点击仍打开详情。
- 我的页与首页帖子图片无白边黑边、不裁切（方形化例外），多图等高于容器且可横滑；单图按原比例。
- 卡片标题、正文、图片三段间距明显收紧且无溢出。
- 第二轮：爱心点亮为红色、收藏为蓝色；首页与我的页评论图标一致且都打开帖子（首页不再弹快速回复）；多图左右出血而单图不变。
- 第三轮：下滚后左右切页签，整体滚动位置与组件位置不变、不跳顶部、切回原页签不跳旧深度；点图片从缩略图位置放大到整屏；互动按钮有简洁点击反馈与数量动画。
- 第四轮：图片下边距与卡片下边距一致；首页与我的页卡片同一组件、同一版式，首页独有信息与入口保留；放大过渡不受无障碍设置屏蔽；切页签时若目标内容未就绪则等内容到位再换，位置始终不动。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 实现 | 工作区未提交改动（`identity_avatar_flow.dart`、`personal_profile.dart`、`post_images.dart`、`post_screens.dart` 及两份测试） | 四项均完成，首页/我的页图片共用同一实现 |
| 验证 | Flutter 3.47.5（`C:/Users/Poeticlight/flutter_3_47_5`）＋未提交差异 | `flutter analyze --no-pub` 0 问题；`feed_media_test`、`personal_profile_test`（含新增两点用例）、`personal_reposts_integration_test`、`post_screens_test`、`community_localization_test`、`post_images_test`、`post_controller_test`、`feed_interactions_test`、`feed_ui_test`、`post_edit_screen_test`、`post_sharing_test`、`profile_settings_flow_test`、`settings_screen_test` 全部 PASS；裁剪与真机可见效果 NOT_RUN |
| 第二轮验证 | Flutter 3.47.5＋未提交差异 | `flutter analyze --no-pub` 0 问题；`personal_profile_test` 11 项 PASS（含评论打开帖子＋红心、切未访问页签落点）；上一轮 `comment_views_test`／`feed_media_test`／`feed_ui_test`／`post_screens_test`／`personal_reposts_integration_test`／`community_localization_test` PASS；首页快速回复用例按新行为改写。全量回归与真机 NOT_RUN。 |
| 第三轮验证 | Flutter 3.47.5＋未提交差异 | `flutter analyze --no-pub` 0 问题；`personal_profile_test` 11（含“切页签保持深度滚动位置”）、`post_images_test` 23、`feed_media_test` 14、`navigation_motion_test` 13、`comment_views_test`／`feed_ui_test`／`post_screens_test`／`personal_reposts_integration_test`／`community_localization_test` 142、`talkback_semantics_test`／`interaction_motion_test`／`post_reposts_test`／`feed_interactions_test` 47 全部 PASS。`packages/image_viewer` 自带测试 NOT_RUN（本机缺缓存依赖 `image`，`pub get --offline` 失败）；真机动效观感 NOT_RUN。 |
| 第四轮验证 | Flutter 3.47.5＋未提交差异 | `flutter analyze --no-pub` 0 问题；单文件顺序执行：`feed_ui_test` 41、`personal_profile_test` 11、`feed_interactions_test` 28、`comment_views_test` 67、`post_screens_test` 29、`talkback_semantics_test` 4、`community_localization_test` 4、`navigation_motion_test` 13、`interaction_motion_test` 3 全部 PASS。同步修正的测试：首页时间改为显示相对时间、旧 `HhXPostItem` 内省改为 `PostFlatAction`、动作顺序 comment→like→bookmark；顺带补上首页点赞/收藏回调的 `!current` 守卫（旧账号失效后不得再发写请求）。真机动效观感仍 NOT_RUN。 |
| 环境 | 本机 C: 磁盘 | C: 曾 100% 满（0 可用）导致 Gradle/`flutter test` 写临时文件失败；已清理 `%LOCALAPPDATA%\Temp`、`.gradle/.tmp`、`.gradle/daemon`，现约 4.4G 可用。Android 构建前建议再腾出数 GB（如 `.gradle/caches` 3.5G，可重新下载）。 |
| 第五轮验证 | Flutter 3.47.5＋未提交差异 | `flutter analyze --no-pub` 0 问题；`post_screens_test` 29、`feed_interactions_test` 28（动作顺序改为 like→bookmark→comment）、`feed_ui_test` 41、`personal_profile_test` 11、`talkback_semantics_test` 4、`interaction_motion_test` 3、`community_localization_test` 4 全部 PASS。真机观感 NOT_RUN。 |
| 第六轮验证 | Flutter 3.47.5＋未提交差异 | `flutter analyze --no-pub` 0 问题；临时探针确认首页两张卡片首尾相接（281→281）、列表无分隔节点、首页横向三点 2 个/竖向 0 个；`feed_ui_test` 41（入场动效用例补 1 张前置卡片，因卡片变矮目标卡原已进入视口）、`feed_interactions_test` 28、`personal_profile_test` 11、`post_screens_test` 29、`talkback_semantics_test` 4 PASS。真机观感 NOT_RUN。 |
| 第七轮验证 | Flutter 3.47.5＋未提交差异 | `flutter analyze --no-pub` 0 问题；临时探针确认空页签 `maxScrollExtent=0`、切换后残留消失并稳定在 0；`personal_profile_test` 12（含新增空页签用例）、`post_screens_test` 29、`interaction_motion_test` 3、`feed_interactions_test` 28、`comment_views_test` 67 全部 PASS。真机观感 NOT_RUN。 |
| 第八轮验证 | Flutter 3.47.5＋未提交差异 | `flutter analyze --no-pub` 0 问题；探针：空页签 `maxScrollExtent=301`、深滚 530 切换后停在 301、继续下滑保持 301、空状态文字位于空白区中心；`personal_profile_test` 12（空页签用例改为断言“停在头像收起位置且再滑动不变”）、`post_screens_test` 29、`feed_interactions_test` 28 PASS。真机观感 NOT_RUN。 |
| 第九轮验证 | Flutter 3.47.5＋未提交差异 | `flutter analyze --no-pub` 0 问题；`app_navigation_test` 4、`navigation_motion_test` 13、`login_home_navigation_test` 18、`feed_scroll_refresh_test` 15、`personal_profile_test` 12、`talkback_semantics_test` 4 全部 PASS。真机观感 NOT_RUN。 |
| 第十轮验证 | Flutter 3.47.5＋未提交差异 | `flutter analyze --no-pub` 0 问题；`app_navigation_test` 4、`personal_profile_test` 12、`talkback_semantics_test` 4 PASS。预览夹具仍无 `CommentRuntime`，回复页签在预览里会显示居中后的“回复服务暂不可用”（正式应用有运行时，不出现该文案）。真机观感 NOT_RUN。 |
| 第十一轮验证 | Flutter 3.47.5＋未提交差异 | `flutter analyze --no-pub` 0 问题；临时用例确认预览夹具下「回复」页签渲染出回复内容且不再出现“回复服务暂不可用”；`personal_profile_test` 12、`app_navigation_test` 4 PASS。真机预览观感 NOT_RUN。 |
| rebase 后复验 | 同上＋`dbbc249`（origin/main） | 解冲突：`post_screens.dart` 一处（保留上游 `_AuthorPostCard.source` 结构，时间调用改用本项目改名后的 `postDateLabel`，删除旧 `_PostCardWidget.meta` 的图片数/浏览量两行）。`flutter analyze --no-pub` 0 问题；`post_screens_test` 33、`feed_ui_test` 43、`personal_profile_test` 12、`feed_interactions_test` 28、`comment_views_test` 67、`feed_media_test` 14、`personal_reposts_integration_test` 1、`app_navigation_test` 4、`navigation_motion_test` 13、`interaction_motion_test` 3、`talkback_semantics_test` 4（共 222 项）PASS；`feed_media_test` 的预留高度断言按新间距由 `+8` 改为 `+2`。 |
| 第十二轮验证 | Flutter 3.47.5＋未提交差异 | `flutter analyze --no-pub` 0 问题；探针：深滚 530 → 1 条回复的页签停在 301（`ext=301`）且再滑动不变，空页签同；`personal_profile_test` 12、`personal_reposts_integration_test` 1（下拉刷新用例先回顶部再下拉，因尾部空白补足改变了拖动语义）、`app_navigation_test` 4、`feed_ui_test` 41、`talkback_semantics_test` 4 PASS。真机观感 NOT_RUN。 |
| 预览夹具 | 同上＋`lib/dev`（本地排除目录） | `flutter analyze` 通过；临时宿主用例确认预览假 API 下爱心可写入、三点弹出“编辑帖子／删除帖子”、预览图能生成为正确尺寸的 `MemoryImage`（用后即删）；预览页面真机观感 NOT_RUN |

## 阻碍与下一步

用户已在真机确认全部轮次的效果（“好了都解决了”），实现与验证均完成。下一动作：按逻辑拆分提交 → rebase 到最新 `origin/main`（上游 `#21/#22/#23` 动过 `post_screens.dart` 与 `feed_ui_test.dart`，需解冲突并复跑受影响测试）→ 推送分支并创建 PR；合并与发布需用户另行授权。未覆盖项：裁剪几何无既有自动化测试（依赖真机确认）、`packages/image_viewer` 自带测试本地缺缓存依赖未运行、iOS 与远端 CI 待 PR 后执行。
