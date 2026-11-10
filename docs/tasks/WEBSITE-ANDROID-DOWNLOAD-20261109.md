# WEBSITE-ANDROID-DOWNLOAD-20261109 — 官网 Android 扫码直接下载

- 模式：常规；当前 Session 维护；无委派。
- 分支／Worktree：`codex/feat/website-android-download-20261109`；`E:/Hnuhole-env/source/SpeechFUL-website-android-download-20261109`。
- 实际基线：已部署官网来源 `59249d565b46e39624d22830f19facd78f7853e5`，与用户选定官网 ZIP 的 index.html 摘要一致；官网源码尚未在 main，故保留该实际部署依赖，不以 main 覆盖官网。不修改原官网各分支及其已有改动。
- 授权与确认：用户要求在官网 Android 对应区域放二维码，扫码下载最新安装包。经 grill-with-docs 确认直接展示，用户进一步明确“只有二维码，不需要下载按钮”“二维码扫了以后跳转下载”。采用二维码直接指向固定 HTTPS APK 路径，不经过介绍页；后续换版本不换二维码，iOS仍未开放。无未决产品取舍。
- 状态：实现和聚焦验证PASS；码云v1.0.0公开附件、固定官网跳转及可点按二维码已正式部署，公网复验PASS；源码已提交[PR #127](https://github.com/zewbby/SpeechFUL/pull/127)，合并结果以该PR实时状态为准。无当前发布阻塞。
- 更新日期：2026-11-10。

## GitHub合并及入口可用性补验

用户2026-11-10明确要求“把这次官网和下载入口的改动也更新到GitHub，并合入main”，授权本任务推送、创建PR及合并。核对main尚未包含官网源码，因此包含已部署官网依赖链及本次下载入口，保留main其他改动。用户随后提供仍显示“暂未开放”的截图，真实用户端未通过；此前独立浏览器PASS不代表该端通过。原实现依赖运行时fetch清单才能解锁已发布入口；具体内置浏览器失败原因尚未确认。确定性修复为直接在HTML呈现已发布二维码、版本和链接，取消运行时清单依赖，构建检查核对HTML与清单一致；iOS仍未发布。禁脚本及屏蔽清单的本地与真实公网点击下载均PASS，文件名SpeechFUL.apk；证据static-download-local-proof.json、static-download-live-proof.json。修复产品提交ad6fa90，整合main后被测head219792107253f10630808e055688ad377fefe5d0，生产部署https://4a04f3d1.speechful-official.pages.dev。该head的GitHub Website产品CI已PASS；完整CI及合并状态见PR #127。

## 目标与依赖

官网 Android 卡片显示可点按的下载二维码和加粗版本，没有额外下载按钮；用户后续明确“也可以不用二维码，怎么方便怎么来”，接受国内直接下载入口。二维码和点按链接使用官网固定地址`https://www.speechful.dpdns.org/downloads/SpeechFUL.apk`，由Pages的302规则跳转已验证的码云公开APK，后续换版本无需更换二维码。不使用外部二维码服务，不请求业务API。安装包为已发布且同签名真机升级通过的 SpeechFUL v1.0.0，Android 10+／ARM64，37,177,322字节，SHA256 `2deba24136662daa86458c7bcb754edd95e4762ac425c6d32fbc8397f82f5d8f`。

Cloudflare Pages单文件上限25MiB，APK超过上限，因此官网下载页面与APK分开托管。码云直链为 `https://gitee.com/axibass/speechful-android-release/releases/download/v1.0.0/SpeechFUL.apk`，新版官网下载不依赖本机分发文件。为兼容已有二维码，旧固定地址 `https://speechful.dpdns.org/downloads/SpeechFUL.apk` 改为302跳转该码云地址；原版本路径、beta下载、C/V业务代理及全部认证／Gate不变，不修改数据库或重启业务程序。旧固定地址的兼容跳转及业务API仍使用当前公网托管电脑。

## 原固定地址版本的验证与交付

- PASS：独立QR生成后用zxing解码，内容精确为上述固定APK地址；不是Release页或官网介绍页。
- PASS：Node语法、网站资源／配置／静态回退检查、原演示12项回归、静态构建。仅修改官网下载相关文件，不重新执行Android／后端全量。
- PASS：Nginx配置测试后正常reload，固定路由指向与GitHub和手机相同字节的v1.0.0 APK；附件文件名、类型、no-store及nosniff，不开放目录列表。`E:/Hnuhole-env/website-android-download-20261109/apk-route-proof.json`。
- PASS：公开固定APK路径200、正确附件类型与文件名，完整GET 37,177,322字节，SHA256与已装机／GitHub版本一致；首次读取在4,704,175字节中断，聚焦重试一次完整通过，保留该网络异常。`public-download-proof.json`。
- PASS：已有Edge独立离屏会话下320／390／1440px，二维码可见、页面无横向溢出及脚本错误、Android零按钮或链接、iOS保持未开放；三种尺寸的实际渲染二维码均由zxing解出同一固定APK地址。`page-proof.json`、`rendered-qr-proof.json`、`card-*.png`。
- PASS：可上传官网静态ZIP共27文件、8,324,583字节，根目录为index.html，不含APK；ZIP内容逐文件对比构建输出一致。SHA256 `8076f66b66d9bdb3f1ad4cad9e4be35a655dca664180a921121139372acd985d`；`package-proof.json`。包名 `SpeechFUL-website-android-QR-v1.0.0.zip`，位于证据目录。
- 证据目录：`E:/Hnuhole-env/website-android-download-20261109`。离屏测试使用已有浏览器独立会话，不捕获桌面或读取已有浏览器会话。

## 部署与变更记录

用户选择“登录授权”，使用Cloudflare Wrangler设备登录由用户自行授权。只读确认项目 `speechful-official` 的现有生产分支为main，部署前27个静态文件与已验证ZIP逐字节一致；现网与原部署源码差异仅Cloudflare自动注入的邮箱保护和统计脚本。使用Direct Upload部署到既有生产项目，未修改DNS；部署地址 `https://51bd1f83.speechful-official.pages.dev`。

PASS：2026-11-10正式官网 `https://www.speechful.dpdns.org/` 的JS、样式、发布清单及二维码图像与已验证构建一致；对公开二维码图像解码得到固定APK地址，公网GET Range返回206及附件头，首1024字节与v1.0.0安装包一致。完整文件下载与SHA校验为2026-11-09的真实公网证据，不冒充本日重复完整下载。iOS清单仍未开放。`E:/Hnuhole-env/website-android-download-20261109/live-publication-proof.json`。未读取电脑界面或用户Cloudflare密码。

### 用户要求调整文案

用户指定Android卡片下方只显示加粗的 `v1.0.0`，iOS下方显示“暂未发布”。已完成源码修改及静态检查；本地离屏浏览器在320、390、1440px核对Android版本文本与700字重、iOS文案、二维码可见及无Android下载按钮，PASS。Cloudflare Pages生产部署 `https://7723a0b5.speechful-official.pages.dev`；正式官网JS、CSS、页面文案、二维码和发布清单复验PASS，`live-wording-proof.json`。二维码地址和APK未改。

用户进一步要求下载包命名SpeechFUL，固定下载路由及版本路由的响应文件名均调整为 `SpeechFUL.apk`，经nginx配置检查和正常reload后，本地／公网Range请求均核对文件名和APK字节一致。用户反馈下载慢：2026-11-10同一256KiB片段本地约0.02秒、公网约1.95秒（约131KiB/s，包括连接开销）；首次公网探针连接失败后复测通过，`apk-speed-proof.json`。安装包目前从用户电脑经现有公网隧道分发，公网链路速度受电脑上行、隧道和接收端网络影响；本次样本不足以确定具体瓶颈。未更改安装包、业务数据或DNS。

### 国内直链候选

用户不希望扫码后进入网盘分享页，不使用GitHub，要求中国大陆普通网络可访问；二维码可不使用。曾考虑Cloudflare R2，账号未开通且可能要求绑定计费，用户选择暂不开通；免费Cloudflare CDN对大文件分发有额外服务条款，不能承诺免费适用。用户同意尝试中国码云Gitee个人免费发行版附件，先验证匿名直链、文件名和速度，再决定是否切换；不修改现有官网入口，直到新路径通过核验。

`SpeechFUL.apk` 已在本任务私有交付目录准备，37,177,322字节及SHA256与v1.0.0完全一致。码云官方说明单发行版附件100MB上限；另一个公开项目匿名APK探针返回200、Content-Disposition含.apk，但Content-Type为application/zip，256KiB单次耗时11.116秒，包含建连开销，不能推断自身APK或其他网络的速度；现有公网同量探针约1.95秒，均仅为单次样本。本机无已保存的Gitee凭据（用户明确同意后只输出有无，不输出凭据），用户已完成码云登录；后续由本机回环表单接收用户自行填写的令牌，仅在进程内存中向码云官方API使用，不写入仓库或文件，不读取用户桌面、密码或验证码。

### 码云授权与发布检查

本机表单初次403由发布助手的`Referrer-Policy: no-referrer`与严格Origin校验冲突引起；在独立离屏Edge会话和模拟令牌下复现Origin为null。修正为same-origin，增加每进程CSRF值，保留严格Origin、Host、大小与类型校验；正常浏览器提交PASS，外部来源、null来源、缺失／错误CSRF均拒绝。证据`gitee-form-test-proof.json`，未向真实码云发送测试令牌。

用户重新提交后，真实调用在创建仓库返回值的公开状态断言处停止，尚未上传APK；旧助手在断言前未保存创建回执，因此不假定创建结果或重复创建。当前码云官方规范`https://gitee.com/api/v5/doc_json`注明个人创建仅支持私有，仓库设置PATCH另有private字段。助手改为先核对本次新建目标的账号、路径、ID、时间、描述、主页、仅README且无Release，再保存无凭据回执；通过官方设置请求公开，重新读取确认公开后才创建Release和上传APK。其他既有仓库或额外内容停止处理，不删除或覆盖。模拟的新建、恢复创建、公开设置拒绝、旧仓库、额外内容、账号不匹配六种场景PASS，`gitee-publication-test-proof.json`；这些模拟不计为实际码云发布或匿名下载通过。

后续真实检查确认初始化仅有README.md及README.en.md，原检查误拦英文README；已修正并增加不含凭据的诊断。模拟检查增为七项，正常提交、安全拒绝及失败后内存授权续用PASS；用户无需在每次重试时重新填写令牌。仓库`axibass/speechful-android-release`（ID 51377334）由本次创建，API设置公开返回422；用户自行在网页设为公开后，助手重新读取确认private=false、public=true，创建v1.0.0（Release ID 1293000）并上传SpeechFUL.apk（附件ID 3649056）。完成后清除内存令牌并停止授权服务，未将令牌保存到文件或聊天。

### 码云实际下载及切换验收

- PASS：无Authorization、无Cookie的公开完整GET，37,177,322字节，SHA256与已发布／已装机版本一致，23.104秒；同机紧接着完整读取原公网地址为58.724秒，摘要相同。本次码云约快2.54倍，仅代表当前电脑／网络，不保证其他网络速度。此前三次256KiB探针码云中位151.5KiB/s、原地址432.0KiB/s，包含建连开销；不能用小片段结果代替完整包结果。`gitee-full-download-proof.json`、`current-public-full-download-proof.json`、`gitee-speed-proof.json`。
- PASS：码云响应Content-Type为application/zip，但Content-Disposition为SpeechFUL.apk；独立无登录Edge实际下载建议文件名确为SpeechFUL.apk。`gitee-browser-download-proof.json`。
- PASS：新版二维码直接解出码云公开APK地址；320／390／1440px实际渲染后均解码一致。二维码可点按，390px实际点击触发SpeechFUL.apk下载；无横向溢出和脚本异常，Android仅有一个二维码链接且零额外按钮，版本v1.0.0字重700，iOS暂未发布／暂未开放。`gitee-qr-proof.json`、`gitee-rendered-qr-proof.json`、`gitee-page-proof.json`。
- PASS：网站CI对应的本地全部命令：三份JS语法、12项演示回归、资源／清单／静态回退检查、构建；系统无npm，直接运行package.json中相同Node命令。不是GitHub远程CI结果。产品改动以基线`c45322b29a54411d346661428f756426f4cf9632`加本次官网五个文件差异定位，后续承载提交绑定相同内容。
- PASS：仅替换Nginx固定APK的精确location，GET／HEAD返回302、POST返回405、no-store，其他配置逐字不变；nginx -t及正常reload成功，C/V就绪各200。原版本和beta下载保留。独立浏览器经旧固定公网地址也触发SpeechFUL.apk下载。`gitee-fixed-route-proof.json`、`fixed-route-browser-download-proof.json`。
- PASS：码云直接链接的官网版本`42522c57f8e8f73460c082219950961d19222b2d`已部署到既有项目speechful-official，部署`https://0e690f15.speechful-official.pages.dev`；正式官网资源摘要、320／390／1440px以及390px真实点按下载PASS，`gitee-live-publication-proof.json`、`gitee-live-page-proof.json`。旧固定地址的默认urllib标识被Cloudflare拒绝403；使用与此前下载检查一致的验证程序标识后302通过，真实无登录浏览器也通过。未将该403计为下载通过。

为保留最初确认的“固定下载地址、后续不换二维码”，最终官网链接和二维码改用www官网的固定路径，以Pages原生_redirects规则302至同一码云附件；文件不经本机中转，也不增加对象存储计费。静态检查核对该唯一规则、固定URL及QR资源，独立解码PASS（`official-download-qr-proof.json`）；行为与此前已验的可点按二维码相同。本次不启用R2、不改DNS、不迁移官网到新托管项目。

最终产品提交`7b1180b08c03c1840933fafdacf459033a90b389`，生产部署`https://4cee44bb.speechful-official.pages.dev`。首次部署请求超时后先核对未产生新部署，再重试成功。PASS：正式官网JS／CSS／清单／二维码摘要与最终构建一致，新旧两个固定地址均302至同一码云APK；320／390／1440px页面及390px真实点按下载SpeechFUL.apk通过，三个尺寸实际渲染二维码均解出www固定官网地址。证据`official-download-live-proof.json`、`official-live-page-proof.json`、`official-live-rendered-qr-proof.json`。同一APK的完整SHA及速度证据仍适用，未重复构建或装机。源代码本地提交、产品检查PASS、生产部署PASS、合并未执行；GitHub远程CI为NOT_RUN（无本任务PR），不冒充远程通过。
