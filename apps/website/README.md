# SpeechFUL 官网

静态 HTML/CSS/JavaScript，无安装依赖，使用 Node.js 22 或更高版本。在本目录运行（仓库内路径为 `apps/website`）：

```sh
node scripts/serve.mjs
```

打开输出的本机地址，默认 `http://127.0.0.1:4173/`。源码修改后刷新即可。PowerShell 可指定其他端口：

```powershell
$env:PORT = '4183'
node scripts/serve.mjs
```

本次协作预览使用 4183。服务器仅监听本机，不会发布到互联网。

## 检查与构建

```sh
node --check public/site.js
node --check public/demo.js
node --check public/demo-playback.js
node --test scripts/demo.test.mjs
node scripts/check.mjs
node scripts/build.mjs
```

也可用 `npm run check`、`npm run build`、`npm run dev`。构建结果位于 `dist`，部署时只需该目录。仓库的 `.github/workflows/website.yml` 运行同一检查和构建；发布与合并状态见任务记录及其关联PR。

## 页面与演示

- 首屏按 README 的上下顺序组织图案、字标、理念、两行题词和获取 App 入口。
- 全页纯白底，黑色文字、蓝色动作；Android 绿色与 Apple 黑色保留。模块标题字号统一。
- @SpeechFUL 由同一 Roboto Black Italic 字体生成轮廓；普通 @ 做光学大小与基线调整。字体和图案本地加载，无外部字体服务。
- Logo 扫光、嘘手势与轻微文字擦亮在入场播放一次，可点击／悬停／聚焦重播。减少动态偏好下停用。
- 关于我们、下载和联系内容进入视口时分段显现，移出后再次进入可重播；减少动态偏好下不播放。页面内容默认可见，不依赖脚本或滚动动画才能阅读。
- “关于我们”右侧是离线 HTML 交互模拟，不是真机视频。按 App 界面展示欢迎、信息流、快捷回复、帖子详情、通知和私信；首次私信展示等待对方回复的状态，随后由虚构对象固定回复一次。进入视口后自动演示一次；离开视口或页面隐藏会暂停，返回后继续。点击步骤、聚焦手机或输入时接管自动流程，可手动操作。
- 评论、通知状态和私信只保存在当前页面内存，刷新后清空；不连接业务、认证或消息 API，也不写入浏览器存储。所有示例人物和内容均为虚构。手机内部使用 App 原有 Roboto、Noto Sans SC 字体及欢迎页字标；登录流程在演示中略过。
- 手机是参照黑色 iPhone 18 Pro Max 的 CSS 展示模型，使用约 78:163.4 机身比例、小号灵动岛和黑色边框厚度，不是制造用 CAD 或真机兼容性证明。参考：[Apple 技术规格](https://support.apple.com/en-lamr/148591)、[官方产品介绍](https://www.apple.com/newsroom/2026/09/apple-debuts-iphone-18-pro-and-iphone-18-pro-max/)。
- 联系模块提供建议说明与邮箱；右下角箭头返回顶部。FAQ、冗余介绍、旧场景卡和页脚已删除。

源码：`public/index.html` 为结构，`style.css` 为全站样式，`demo.css` 为手机及演示样式，`demo.js` 为本地交互，`demo-playback.js` 为有限播放控制器，`site.js` 为导航和品牌动画，已发布下载入口直接呈现在HTML。

资源出处和许可证见 `public/assets/NOTICE.txt`。仓库权威任务记录位于 `docs/tasks/WEBSITE-01.md`，独立源码包不包含整个 App 仓库。

## 50 个 App 的设计分析

打开 [DESIGN-RESEARCH.html](DESIGN-RESEARCH.html) 查看按下载热度来源顺序整理的 50 个样本、四项评价、方法限制与应用结论。`design-sources.json` 保存来源清单。排名为公开累计下载参考榜原名次，不是当前月度榜，也不是设计评分；每条应用名称链接到官方公开界面证据。

## 安装入口

`public/releases.json` 与静态HTML共同记录发布信息，构建检查核对二者一致。Android 已开放 SpeechFUL v1.0.0（Android 10+／ARM64），在原平台卡片展示二维码，不增加下载按钮；iOS仍未开放。

二维码资源 `public/assets/android-download.png` 精确编码固定地址 `https://www.speechful.dpdns.org/downloads/SpeechFUL.apk`，扫码直接获取APK附件。后续版本沿用该地址，只更新 `public/_redirects` 中已校验的码云附件目标，再同步清单和HTML版本号；无需更换二维码。二维码由本地生成，不调用外部二维码服务。

```json
{
  "available": true,
  "version": "1.0.0",
  "url": "https://www.speechful.dpdns.org/downloads/SpeechFUL.apk",
  "qr": "assets/android-download.png"
}
```

仅接受不带用户名或密码的 HTTPS 地址。已发布Android二维码和链接不依赖JavaScript或运行时清单请求；未发布iOS仍显示“暂未开放”。本地演示手机采用 iPhone 外观不代表 iOS App 已发布。APK超过Cloudflare Pages单文件25MiB上限，不随官网静态ZIP上传；安装包由码云公开Release提供，官网Pages仅进行302跳转，下载不依赖本机在线；旧非www地址仅作兼容跳转。任务状态见 `docs/tasks/WEBSITE-ANDROID-DOWNLOAD-20261109.md`。
