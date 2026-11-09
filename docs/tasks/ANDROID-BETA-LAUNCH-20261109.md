# ANDROID-BETA-LAUNCH-20261109 — 最新主线 Android 公网内测上线

- 模式：常规；负责人／状态维护人：当前 Session；无委派。
- 分支／Worktree：`codex/chore/android-beta-launch-20261109`；`E:/Hnuhole-env/source/SpeechFUL-android-beta-launch-20261109`。
- 基线：远端 main `f9388e3f979b8d0e34b0d4f2b5b7c0b98c662a33`，2026-11-09 实际 fetch 核对。
- 授权依据：用户明确要求“优先把最新 main 的 Android 内测版本上线，完成后端升级、打包和公网真机验收，有阻塞及时告诉我”。覆盖既有公网试用环境的备份、前向迁移、服务升级、同包签名打包及自有测试账号验收；不覆盖 iOS、购买服务、清库、绕过 Gate、操作真实用户账号或向其他人发送消息。
- 需求确认：沿用本聊天已接受方案，Android 优先、少量受邀用户内测；保留现有公网域名、独立 C/V、现有数据及安全边界，Passkey／恢复码维持最新 main 暂停策略。用户随后明确只聚焦修改项，不重跑历史全量。
- 本轮交互修正：用户指出欢迎／登录页常驻“查看处理通知与申诉”不符合预期，明确“如果是被封号了只在登录的时候显示账号已被封禁然后出现申诉/取消按钮”；经 grill-with-docs 事实核对及集中提问，用户选择“复用专用申诉登录页”。移除两个常驻按钮；正确凭据＋最终 Gate 确认封禁才弹窗，错误密码不透露封禁，取消留登录页，申诉进入既有独立 MODERATION_SELF 登录。无未决取舍；本轮新增补丁尚未发布，不将旧包验收提升为新补丁通过。
- 注册反馈修正：用户实际校邮确认后遇到通用“操作未完成”，进一步明确“如果是注册过…要提示该邮箱已注册”。V元数据为已消费验证码、已存在配额；客户端将已验证后的权威 ELIGIBILITY_RESERVED 拒绝显示为“该邮箱已注册”，不改发码／验证／资格流程、不删除原键、不把其他拒绝或未知结果改成已注册。新增确认＋原结果查询回归PASS。
- 更新日期：2026-11-09。
- 状态：PR #124 已获用户明确批准并 Squash 合并，main `4d12c8d5277199df7cde487ca3477f2203f5eb09` 与通过14项CI的候选 `f6b9eff` 树一致。beta.4 后端已部署并恢复可用；管理员已初始化。审核网页空白的默认HTTPS端口匹配修复、快照发布完整性修复待聚焦验证／新PR；最终内测发放待验。

## 目标与范围

固定上述 main，备份并升级既有 PUBLIC-TRIAL-ACCESS-01 环境到 C38/V8，构建可覆盖升级的签名 Android APK，完成公网真实服务及真机业务验收后提供内测交付。既有历史验收按原版本保留，本记录维护本轮发布状态。

## 验收标准

- 备份包含 C/V 数据、配置、密钥／Gate 锚、媒体及旧程序；备份私有保留并检查可读性，不在 Git 或聊天暴露秘密。
- 前向迁移及精确权限更新；原账号和业务数据保留、Gate 状态／代次和恢复审计不因升级被重置。
- APK 绑定源码及打包差异、固定公网地址、原 publictrial 包名与签名、递增版本号；全新安装／覆盖升级按设备条件分别验证。
- 公网注册／登录／校邮重置、冷启恢复、图文内容及私信、举报／处理按自有合成账号完成；不把健康检查或替身测试当真机通过。
- 记录运行恢复、下载交付及实际未验范围；缺真机或真实邮箱条件时保持待验。

## 当前进度与证据

| 项目 | 环境／被测版本 | 结果 |
| --- | --- | --- |
| 基线 | main f9388e3，PR123 已合并且产品树与通过 CI 的 head 一致 | PASS；本轮 fetch 确认，无源码修改 |
| 备份／升级演练 | 独立 Linux 私有目录，两个新建 PG16 恢复库 | PASS：C35/V7 备份恢复至 C38/V8、账号和帖子数量保留、基础权限审计；可选DM权限随后按既有流程启用，实际启动门槛另验。首轮误把完整DM权限放在基础审计前而FAIL，已按发布工具的既定顺序修正；未放宽审计。`restore-upgrade-rehearsal.json` |
| 一致冷备份 | 只停止本任务两个systemd服务及专属监控；PG顺序停止 | PASS：运行目录全量冷归档、C/V逻辑备份可列出、旧源代码／程序、配置／密钥／Gate锚／媒体均私有保留。`backup-receipt.json`；源状态标签历史滞后，旧二进制以归档内容为准 |
| 实际部署 | main f9388e3；C38/V8 | PASS：实际C/V进程摘要匹配固定构建；账号3、原帖4保留，Gate OPEN C1/V2及RECOVER审计1/2不变，配置摘要不变，无recover。`upgrade-receipt.json`、`process-proof.json`、`runtime-after.json` |
| 公网API | 两个正式域名、正常TLS；自有合成账号 | PASS：登录、feed／搜索／热榜、暂停旧恢复、无凭据拒绝、定向登出及旧会话拒绝。一次PC网络TLS EOF保留为异常观察；后续完整链通过，不声明广泛外网稳定性。`local-smoke.json`、`public-smoke.json` |
| 图片业务API | 原任务键、正常公开API及真实发布worker | PASS：图片申请／上传、真实发布、内容读回、原图SHA一致、本人删除及404；只新增并删除本任务合成帖，数据库保留删除终态，故总帖记录4→5。首次上传403，沿原键统一正常客户端标识后通过，未改安全配置，403精确来源未证明。`public-write-smoke.json` |
| Android主候选 | f9388e3＋包名／标签包装；release/ARM64，2026110901，minSdk29 | PASS：原publictrial包名／签名，37,177,338字节；SHA256 `4397d576fed7f39afb7c7a44cf917d63e4bc635a47aa192c0084bd92f0c57c86`。vivo V2323A Android15 install-r，实际base.apk同摘要、首次安装时间保留；无ADB reverse。原账号保留，未退出／改密。`android-build-receipt.json` |
| 普通服务恢复 | 升级后一次systemd正常restart、原监控恢复Running | PASS：两个公网入口及合成账号认证／读取／登出，Gate和恢复审计不变。紧跟systemctl返回的首个SQL探针因PG尚未就绪FAIL，待启动后复查通过；不是Gate故障。未重启整台Windows |
| 入口修正 Flutter | 基线f9388e3＋本任务明确差异；Flutter3.47.5 | PASS：analyze零问题，六文件143项；随后新增真实TLS客户端／校邮封禁2场景，对应两文件56项PASS（与前143项存在重叠，不累计）。`patch-analyze.log`、`patch-auth-tests.log`、`patch-additional-tests.log` |
| 入口修正 Go／SQL | Linux独立源码、Go1.22.2、一次性PG16 | PASS：authprivacy／http／runtime主机测试、vet、cmd构建；race下4个真实SQL顶层测试全部通过：正确/错误密码与账号状态、冻结Gate不泄露、既有校邮登录、校邮封禁不签发会话。`patch-go-host.log`、`patch-go-vet.log`、`patch-go-build.log`、`patch-real-sql.log` |
| 契约 | `python -B packages/openapi/check-specs.py` | PASS：13份完整规范；ACCOUNT_BANNED只用于验证后的封禁拒绝 |
| 管理员 | 用户确认实际登录用户名zewbbu，昵称zewbby | PASS：用户明确选择保留账号并设管理员，执行canonical首位管理员bootstrap且审计存在；未注销／改密。网页实际管理员登录仍待验。`admin-bootstrap-zewbbu-receipt.json` |

证据目录：`E:/Hnuhole-env/android-beta-launch-20261109/`；Linux 私有材料置于本任务 Linux 目录。

## 阻碍与下一步

手机已连接；用户已通过正常校邮登录，实际用户名与管理员目标确认完成。主包待beta.4覆盖升级；网页登录待Origin修复上线，随后用户自行输入现有密码完成管理员登录。用户要求只复验本轮修改项，历史全量不重复。

### 2026-11-09 后续进展

- 入口修正承载于 f6c9be1，[PR #124](https://github.com/zewbby/SpeechFUL/pull/124)，适用产品CI运行中。原代码的真实SQL、Go、Flutter证据范围如上；仅注册提示增量另验。
- 同源beta.2候选2026110902已构建：publictrial主包release、37,177,342字节，SHA256 `c07d4588780284359c4508d06d5539f30ebceb860024803252a1bb931c37aff5`，未安装；同签名publictrialverify独立验收包已同包升级至2026110902，真机欢迎页／登录页确认无常驻入口。
- 用户指定管理员拟用zewbby；实际登录用户名不存在，未授予权限。用户提供本人可收码校邮，正常注册申请一轮OTP，投递SENT，用户自行输入后V为CONSUMED／0失败、配额已有，C账号仍3，未新增账号。前述用户名不存在不代表邮箱未注册；没有覆盖旧账号或修改密码。
- 用户选择通过校邮登录核对现有账号；已在正常校邮LOGIN流程新发一轮OTP，未复用REGISTER用途。当前手机交用户自行输入，期间停止读屏／点击；成功后再核对当前账号目标，管理员初始化仍待目标确认。
- 用户已确认“登陆好了”；独立验收包正常校邮登录进入既有账号，V为LOGIN/CONSUMED/0失败、投递SENT，C账号数量仍3；当前身份昵称为zewbby，不把昵称误认成登录用户名。核对中发现暂停旧恢复流程同时隐藏了账号与安全页的本人用户名，已恢复该只读显示（仍不展示恢复码／Passkey入口）；6项安全管理页面回归及最终analyze PASS，见`username-tests.log`／`username-analyze.log`。接下来在正常UI核对真实用户名，再请用户确认管理员目标。

### beta.4 部署及审核页故障

- 用户明确批准“合并 PR #124 并继续上线”；合并main `4d12c8d` 与候选 `f6b9eff` 的Git tree均为`9007bd9fd7409130b02090b968d7e5a248c09111`。最终14项产品／协作CI成功，证据`release-ci-proof.json`。后端实际进程摘要匹配`release-runtime-prepared.json`，见`release-process-proof.json`；C38/V8、账号3、帖子5、Gate OPEN C1/V2和RECOVER审计1/2保持。`release-local-smoke.json` PASS真实认证／读取／定向登出；未冒充公网或真机。
- 最终原publictrial主包beta.4/2026110904已构建，SHA256 `e903295482c34a1ce05190b3ae390989f68bd642908f9a10bb0dad152a9ca4c2`；独立验收包同源同签名、真机显示本人用户名zewbbu。主包安装／下载发放仍待完成。
- 用户选择保留zewbbu并设为管理员，已执行canonical bootstrap（仅此账号、首位管理员审计），未提交注销或改密。
- 用户普通Edge与内置浏览器均只有页头；真实Edge DevTools确认`/admin/app.mjs`为403。部署allowedOrigins配置带`:443`，浏览器Origin省略该默认端口；本地实际HTTP探针分别为无Origin200、默认端口省略403、显式443为200、未知来源403。`admin-origin-before.json`。直接改配置为无端口被运行器显式端口校验拒绝（FAIL），已恢复原配置；改为HTTP边界承认同一HTTPS authority的默认端口序列化，其他scheme／host／非默认port仍拒绝。
- 重启另暴露既有评论快照缓存账本损坏：274字节、前273字节均0末字节2、TTL=-1，控制水位已过期、无任何活跃snapshot data key。未绕过完整性检查。先私有备份，仅清理这一条无效派生缓存，控制元数据、全部SQL数据及Gate不变，恢复beta.4服务；`cache-repair-receipt.json`。形态与SETRANGE在缺失key上创建零填充无TTL账本相符，精确历史触发时序未证明；补发布后的绝对过期时间校验，失配沿原save路径恢复完整账本及TTL，并以故障注入模拟precheck后key消失。独立真实Redis聚焦回归待验。
- 先前`local-smoke.json`在本次启动失败时被旧探针覆盖为FAIL，当前候选证据单列`release-local-smoke.json`；历史f938公网及图片证据仍保留原版本范围。诊断只在私有源码副本／日志，未把诊断输出加到发布程序。
