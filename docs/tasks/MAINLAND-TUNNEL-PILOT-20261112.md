# MAINLAND-TUNNEL-PILOT-20261112 — 国内公网通道试验

- 模式：常规；负责人／状态维护人：当前 Session；无委派。
- 分支／Worktree：`codex/test/mainland-tunnel-pilot-20261112`；`E:/Hnuhole-env/source/SpeechFUL-mainland-tunnel-pilot-20261112`；基线为本地 `origin/main`：`9937638c077bf2da7d51daa5de13a83645dc4320`。
- 授权依据：用户确认国内 HTTPS 穿透＋本机后端方案，允许筛选收费方案；确认全校学生取得安装包并通过校内邮箱验证后可使用，预计 100 人以上同时在线、月预算 50 元以内；最新指令要求聚焦技术接入。
- 授权边界：初始为独立合成传输对照；用户后续明确授权接入真实后端、修复启动与 IDEA 工具，并分别批准 V Gate 第 2→3 代恢复及两个 Redis default 回放账号的限定离线修复。保留账号／帖子 SQL、业务账号 ACL、认证检查、官网 Cloudflare 与系统网络设置；预算不作为购买授权。2026-11-13 最新指令为暂不打包，待其他 PR 合并后统一新版本；未授权本分支 Push、PR、合并。
- 需求确认：依据 `grill-with-docs` 及其依赖完成参与范围／负载／预算确认。100 人在线是目标使用场景，不等同 100 个同时上传请求。测速不代替真实 App、SQL、设备或百人容量验收。
- 更新日期：2026-11-13。
- 状态：国内合成传输、WSL 后端恢复、IDEA 启停／状态／日志工具与上传代理诊断已实施；聚焦本地验证 PASS；Frp 健康检查有一次超时，随后恢复，稳定性待验。手机现装旧 App 仍用旧域名；Frp 真机图片成功闭环／完整认证／百人容量 NOT_RUN；未合并、未发布、未打包。

## 目标与范围

比较国内节点的请求往返、合成数据上传和下载耗时，在接入真实业务前判断是否值得替换现有传输路径。接收器只绑定回环地址，不读取本地目录或业务数据；临时随机路径、6 MiB 请求预算、8 个处理槽位、最长 15 分钟生命周期。现有上传诊断证据沿用 [原记录](post-upload-diagnostics-20261112.md) 的被测版本范围。

## 接口与依赖

- `tools/mainland-tunnel-probe.py serve --info <新的私有记录路径> --lifetime 300`：分配回环端口并输出端口／生命周期；随机路径能力仅写入私有记录。
- `tools/mainland-tunnel-probe.py probe --info <同一记录> [--origin https://实际穿透地址:端口] [--bytes 5679735]`：三个小请求、一次下载、一次上传；严格校验证书、拒绝跳转、核对传输摘要，无自动重试。默认 128 KiB。
- 账号、节点及客户端仍需就绪。优先验证免费额度；Sakura Frp 白银公开价 20 元/月、36 Mibps、163 GiB/月为纸面候选，不代表实际吞吐或允许该业务正式接入已验收。
- C/V 是独立 HTTPS origin；现有 WebAuthn RP 固定 `speechful.dpdns.org`。新入口需要完整认证和原生 Passkey 验证，不能只凭一个健康页判定可切换。

## 验收标准

- 本地／候选路径收到正确的合成服务标识、完整下载字节和上传摘要回执；记录原始环境与耗时。
- 不持久化上传字节，不提供文件系统读取或任意转发；错误请求不泄露临时能力或业务内容。
- 候选通道先单独测量；百人业务容量、校内网络与移动网络实际体验单列待验。

## 当前进度与证据

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 代码事实 | 基线 9937638 | C/V origin 和独立 RP 约束已复核；业务实现未改 |
| 公网小请求基线 | 当前工具执行环境 | 未形成有效基线：curl 连接失败、外部抓取工具不可访问；不能据此认定用户端服务已停机 |
| 本地合成工具 | 基线 9937638＋本文件及测速工具差异；Windows / Python 3.9，回环 HTTP | PASS：3 次标识核对、5,679,735 字节上传与下载及摘要核对；`local-smoke-result.json` |
| 边界与生命周期 | 同上 | PASS：错误能力路径／文件系统路径 404，超限请求 413，外部明文 HTTP origin 拒绝；30 秒自动退出且记录停止。`local-boundary-result.json`、`local-expiry-result.json` |
| 源码与协作检查 | 同上 | PASS：Python AST、差异空白、暂存协作检查。`local-validation-proof.json` 保存测速源码 SHA256 和证明范围 |
| 国内通道 | 9e4c0c0；Windows / Python 3.9；Sakura 普通用户，长沙电信PLUS2 | PASS：严格公网 TLS、3 次健康请求及 5,679,735 字节双向传输／摘要校验；详见 2026-11-13 记录 |

首轮将 30 秒接收器与客户端拆为多次工具调用，客户端运行前接收器已经按时退出，因此未得到传输结果。改为同一测试控制进程启动服务、执行客户端与检查到期后，上述本地链路全部通过；没有以第一次的超时调度冒充网络故障。本地回环测试不验证公网 TLS、校内／运营商网络、真实图片校验或百人业务容量。

## 阻碍与下一步

账号、国内免费 TCP 隧道及平台子域绑定已就绪，公网合成传输 PASS。原业务入口未切换。下一步在校内与手机移动网络复测，再准备 C/V 独立 HTTPS origin 与原生 Passkey 兼容的业务接入候选。

## 2026-11-13 公网合成传输证据

- 用户完成客户端登录并启动 `speechfulTest`（29838309）；实际节点为长沙电信PLUS2，远程端口 46128、本地临时端口 60689。平台免费子域 `api.h2454089d.nyat.app` 已绑定；客户端截图确认加载平台证书。未购买或续费。
- 被测提交 `9e4c0c02015a84fdd4a649e6bef8d4d56295ef75`，源工具未改；Windows PC 发起，Python 3.9，现有系统 VPN 未改；工具禁用 HTTP 代理环境变量，不能据此宣称底层路由绕过 VPN。
- 首次受限执行环境立即连接失败；允许网络访问后，严格 TLS 请求到达 SyntheticProbe，随后正式合成探针 PASS。初始失败不计为节点网络故障。
- 正式命令：`python -B tools/mainland-tunnel-probe.py probe --info <private-info> --origin https://api.h2454089d.nyat.app:46128 --bytes 5679735`。
- 3 次健康请求：0.157425 / 0.145691 / 0.145317 秒；下载 5,679,735 字节 3.522013 秒；上传同量 4.363105 秒；TLS 与双向摘要校验 PASS。
- 证据：`C:/Users/24237/Documents/ChatGPT/Hnuhole/deliveries/mainland-tunnel-pilot-20261113/public-probe-result.json`（含源码 SHA256）、`domain-binding.png`。临时能力仅在私有 info 文件，不提交。
- 与历史 Cloudflare 同量级合成数据约 89–93 秒相比有明显改善，但不是同一时刻同端点 A/B。手机移动网络／校内网络、真实图片校验、SQL、C/V 认证、原生 Passkey 和百人容量仍 NOT_RUN。测速服务最长 900 秒自动退出；业务未切换、未合并、未发布。

## 2026-11-13 真实后端接入、恢复与运维入口

本节补充后续用户授权与当前事实；前文“业务未切换”是当时合成测试阶段快照。当前 Frp 已接到真实后端，但不能据此宣称手机安装包或完整业务迁移成功。

### 用户确认与实施

- 用户授权把 App 真实服务接到 Frp，官网原 Cloudflare 通道保留；用户不会手动启服务，要求检查 IDEA External Tools，并确认继续修复后端。常规单分支，无委派；代码基线 `9e4c0c02015a84fdd4a649e6bef8d4d56295ef75` 加本任务 `tools/windows/`、CI 与本记录差异。
- Frp 社区 `www-api.h2454089d.nyat.app:53365` → Windows 回环 36197；认证 `www-auth.h2454089d.nyat.app:48552` → 回环 45123；长沙电信 PLUS2、TCP 自动 HTTPS。原 Nginx 继续向 WSL C/V 私有 TLS 转发，保留证书校验与原 Host 设置。未改 DNS、VPN、全局路由；未购买。
- 后端原本无法启动，独立于换隧道：V Gate FROZEN 第 2 代、评论快照持久化分页缓存 ledger 异常及 Redis 7.0.15 的 AOF 回放 ACL 问题。用户分别确认 V 恢复（旧 V 代次凭据失效）和两 Redis 的准确命令范围后实施。
- V 使用既有签名工具恢复至 OPEN 第 3 代，C 保持 OPEN 第 1 代；备份位于 runtime/backup/frp-v-recovery-20261113T020836Z。账号、帖子 SQL 未重置。异常的单个缓存 ledger 已在原 key scope 内备份隔离，未重建 namespace、未 flush、未旋转缓存 epoch；备份 snapshot-ledger-quarantine-20261113T023328Z。
- Redis 只修改 `default off resetpass` 回放用户，同原业务 key 范围；评论快照仅 DEL/EXEC/MULTI/SELECT/SET/SETNX/SETRANGE，排名仅 DEL/PEXPIREAT/SELECT/SET/ZADD。其余业务和 operator ACL 行不变。完整目录备份 redis-snapshot-before-aof-acl-20261113、redis-ranking-before-aof-acl-20261113；旧 Redis 二进制未替换。维护脚本要求显式 `--target-user default --scope-from-user`，不被日常启停调用。
- host 启动处理提前注册退出清理，启动失败也有序停止自己拥有的子进程与 PostgreSQL；不自动恢复 Gate。原 host 脚本另有备份。
- `E:/Hnuhole-env/backend-operations-20261113/` 的独立控制器接管 IDEA 四按钮：“启动 App 后端 / 查看后端日志 / 停止 App 后端 / 查看服务状态”。Start、Stop、Status、Logs 已分别实际验证；Status 失败明确非零，PASS 要求两服务 active 且两本机健康页 200。IDEA 保存后 XML 精确核对通过，未重启 IDE 或关闭用户工作。
- 原登录计划任务加载的恢复脚本移除业务后端启停，仅保留原有 WSL／Cloudflare 连接维持；App 后端由新按钮手动启动，Frp 在 SakuraFrp 启动器独立启动两隧道。保留原 Cloudflare 连接器，不声称原域名根页面 200（核对曾为 404）。备份工具／计划任务／原恢复脚本见 backend-operations-20261113/backup-20261113-101819。

### 本次“发帖图片慢且失败”与日志补齐

- 用户明确回答：使用手机原来已安装的 App，没有在修改 Additional run args 后重新编译运行。只读确认 D:/SpeechFUL IDEA 参数已是新 Frp C/V URL；该工作区 HEAD `37c95587837f2d99eda5573845ca125b5924d535`，未修改该工作区。运行参数是构建输入，现装旧包不会因此改变。
- 实际代理在 11:18:47、11:19:53 +08 的图片 PUT 警告仍为旧域名 speechful.dpdns.org；与用户反馈一致。缓冲警告不证明完整收体或具体失败原因。本次没有连接中的 ADB 设备，未获得本次手机超时事件，因此不把历史 15 秒超时证据冒充本次请求证据。
- 发帖原图源码仍共用普通 API 的 15 秒总期限；其传输地址由 communityBaseUri.resolve(path) 构造，许可响应没有另给旧的绝对上传 URL。这排除了该版本中“许可把图片重定向旧域名”的机制；不宣称已修复 15 秒设计问题。
- 用户随后要求“先别打包”，等待其他 PR 合并后统一新版本。本轮未构建、安装、修改 App，不合并任何 PR，不重发真实帖子或图片。
- 原“查看后端日志”只看 systemd 启停，实际 C/V 日志位于私有文件、代理 access_log off，无法事后追踪完整上传阶段。本轮新增上传专用元数据日志：仅社区发帖受理／状态／许可／原图固定路由，Host 映射为旧域名／Frp／本机／其他，记录完成标记、长度、HTTP 状态、request ID 与 upstream 分段耗时。没有原始路径、query、客户端 IP、正文或 Authorization。
- Nginx 原配置 SHA256 `6148403a0db3769c73e7c888709517e0e2f33deaa451c2bb436354efc98da3fe` 已精确备份到 runtime/backup/upload-telemetry-20261113；实际 nginx -t、平滑 reload PASS，安装后配置 SHA256 `b3b49d525d1310c427697ed023eb3f106246626d852809e68d167e74b273db07`。只增加条件日志，不改路由／超时／TLS／Gate。日志进本机 system journal，沿用其保留与限流，不提供新公网管理端口。
- 已接到现有 Logs 按钮，增加中文摘要及脱敏 JSON。私有脚本和配置在 backend-operations-20261113/backup-upload-tools-20261113 备份；启停／状态逻辑不变。日志从启用后开始、请求结束才落记录；没有记录不等于上传成功。系统 journal 也不保证完整审计留存。

### 验证与证据

| 项目 | 结果／证明范围 |
| --- | --- |
| V/C、实际 Redis 权限 | PASS：runtime-final-proof.json，C OPEN:1、V OPEN:3；两 Redis 业务 PING 成功、匿名与 default 登录拒绝、scope 外 GET 拒绝；真实实例未执行 FLUSHALL |
| 常规启停 | PASS：正常 Stop → Status FAIL → Start PASS；两本机健康页 200；不自动 recover |
| 源码聚焦回归 | PASS：最终 tools/windows 的 9 项单测，启动失败清理、AOF ACL／解析、日志边界／脱敏与判读；另全新 Linux 临时 Redis 真实 AOF 重启回归 1 项 PASS（现用 Redis 7.0.15），不连接业务库 |
| 工具语法 | PASS：PowerShell Parser；workflow YAML 解析；差异空白检查；GitHub CI NOT_RUN（未 Push） |
| 上传诊断实际链路 | PASS：Frp 32 字节故意无效凭据请求在 journal 记录 frp_domain、HTTP400、completion OK、总耗时0.045秒、upstream400／0.001秒；只证明日志采集和拒绝，不证明真实图片／SQL发布 |
| 当前服务与公网 | 本机 C/V 健康 200；Frp C 200／0.235秒。Frp V 首轮 8秒超时，随后两次严格 TLS 健康 200／0.1765、0.1624秒；短时不稳定证据保留，不能称长期稳定 |
| App 与负载 | NOT_RUN：新包 Frp 真机登录／原生 Passkey／真实发图成功闭环、校园与移动网络、100人容量；用户要求暂不打包 |

证据根目录：`C:/Users/24237/Documents/ChatGPT/Hnuhole/deliveries/mainland-tunnel-pilot-20261113/`；含 runtime-final-proof.json、upload-telemetry-smoke.json（如实保留首轮认证超时）、idea-backend-tools.png。原始私有配置、备份、日志不提交。工具说明见 [Windows 后端工具](../../tools/windows/README.md)。

下一步：等用户所述其他 PR 合并并明确统一构建窗口后，核对最终代码／C/V 构建参数、原图专用期限与状态展示的实际合入情况，再准备统一新版本与真机成功闭环；不自动打包或扩大到其他 PR 的合并。后端 publicOrigin／peerOrigin 和 WebAuthn RP 仍为原配置，完整认证兼容性单列待验，不绕过 Gate、RP 或证书校验。

## 2026-11-16 本机后端 Gate 检查点不一致恢复

- 常规模式，单 Session、无委派。记录分支 `codex/fix/backend-gate-recovery-20261116`，Worktree `E:/Hnuhole-env/source/SpeechFUL-backend-gate-recovery-20261116`；文档基线 `1d829b2623a6c32830ee1b74fae6c99a436ad3b3`。保留 `D:/SpeechFUL` 及其已有 Worktree；本轮不升级运行二进制。
- 授权：用户先问启动失败原因及如何恢复，随后明确“你直接帮我解决这个问题”。仅恢复当前 public-trial 后端并验证；不打包 App、不操作其他 PR、不正式部署。已告知签名恢复会使 C 旧会话及 V 旧代次凭据失效，账号、帖子保留。
- 已确认事实：`resume-c.private.log` 拒绝普通恢复；C 数据库 FROZEN 第 1 代，版本 9510，冻结于 `2026-11-15T23:20:37.593186+08:00`，原因为 `SNAPSHOT_OR_ANCHOR_MISMATCH`。同版数据库高水位 `23:19:29.457707+08:00`、签名 anchor 为 `23:19:29.503424+08:00`。V 数据库仍 OPEN 第 3 代、版本 9501，但数据库高水位 `23:19:29.465466+08:00` 与 anchor `23:19:29.500039+08:00` 也不一致；只恢复 C 不能完整启动。最初为何出现未完成的双端检查点更新尚无确定证据，不将推测记作根因。
- 操作：本机专用脚本 `E:/Hnuhole-env/backend-operations-20261113/recover-gates-20261116.ps1`，默认仅检查；显式 `-Recover` 才在两套私有数据库备份和摘要验证后，调用现有受限身份签名 `authdev recover`。绑定本次冻结时间、版本、代次、资源归属及 authdev SHA256 `7fd55784b7c29ff15c6ddaa52e2f1a72edda85a04b6a1dd85e3aa6e3b2543f14`；不手工写 Gate 行、修改签名文件或重置数据库。备份及原始日志仅存任务私有目录。
- 状态：显式签名恢复、正常启动、常规 Stop → Start、本机／Frp 公网、合成账号登录和受保护读取均 PASS；当前服务保持运行。恢复脚本及本记录为本机成果，未 Push／PR／合并，未正式发布。
- 恢复前先完成两份真实 PostgreSQL custom dump、`pg_restore --list` 目录可读检查及 C/V 检查点备份；备份为 `/home/hnuhole/public-trial-access-01/runtime/backup/cv-gate-recovery-20261116-7df62bb886744448bb620ac8ad525762`，`receipt.json` 记录工具、备份和业务摘要。没有以目录可读声称完整还原演练。数据库只通过各自受限身份及既有签名 `authdev recover` 恢复到 C OPEN:2、V OPEN:4；V 在正常恢复事务中先围栏旧 OPEN 代次。除 Gate／审计以外的数据和序列逻辑导出摘要在恢复前后精确一致，密钥、证书及 C/V／operator 配置摘要不变。
- 验证证据在 `E:/Hnuhole-env/backend-operations-20261113/`：`gate-recovery-20261116-runtime.json` 绑定实际被测 C/V/authdev 二进制与恢复脚本 SHA256；正常 Stop → Start 后仍 OPEN:2/4，累计 RECOVER 数分别 2/4，watch 时间证据继续推进。未修改通用启动工具的“不自动 recover”策略。
- `gate-recovery-20261116-public-checks.json`：本机 C/V `/health/ready` 均 200；实际 Frp 社区／认证入口严格 TLS 均 200。`gate-recovery-20261116-business-checks.json`：已有任务合成账号经 Frp 密码登录 201，当前会话、受保护频道与首页均 200，未登录频道 401。只操作既有合成账号并产生正常新会话；没有发送 OTP 邮件、操作真实用户账号或发布帖子／图片。
- 边界：本次解决已确认的 C/V 检查点不一致及服务不可启动；没有证据认定最初的不完整更新由哪次关机或进程中断引起。没有修改 Gate 的安全一致性规则，也不宣称今后突然断电永不再触发。真机重新登录／上传、Windows 整机重启验收 NOT_RUN；本轮没有 App 或服务二进制升级。

### 2026-11-16 用户反馈后的恢复策略修复

- 用户指出因服务恢复而重新登录不合理，也不应要求其掌握手工恢复。依 `grill-with-docs` 及 `grilling`／`domain-modeling` 核对现有实现后，用户明确选择“按推荐目标修复”：正常重启与短暂故障保留有效登录；崩溃或断电后先自动核对，能证明完整就自动恢复并保留登录，无法确认安全时才暂停服务并明确提示。
- 继续使用同一任务分支与记录。此前签名恢复已完成，不能通过回退授权代次复活旧凭据；此次修复面向后续恢复行为。实现和产品验证进行中，尚无源码修复部署结论。
- 技术边界：仅检查点写入的中断与可能包含业务变更的未知提交必须可区分。计划用每次提交的独立随机检查点标识及签名的只读检查点推进凭据，自动补完可证明没有业务副作用的推进；未知业务提交、真实回滚、签名异常和持久冻结继续 fail closed。证据短暂缺失／合法证据过期只暂时拒绝授权，证据恢复后同代继续；不把暂时不可用直接升级为撤销全部会话。
- 实现：上述机制已写入 Gate 核心，配套 C43／V9 迁移和 V 的单列更新权限；旧版完整匹配的材料同代升级。启动失败将具体 Gate 原因带到服务日志，普通启动保持凭据代次。同步术语、ADR 0007、现有 Gate 契约及适用 Authorization Gate CI；未改 App 或新增登录入口。
- 聚焦验证 PASS：`gate-continuity-focused-tests-r2.log`，新建独占 PostgreSQL C/V 库，`-race` 下真实进程在文件落盘后／SQL commit 前退出、再次中断重放、原登录继续、证据缺失／过期恢复、旧材料升级、业务回滚／错误前序／显式冻结拦截。全后端 build／vet PASS，启动日志元数据回归 2 项、CI 范围回归 4 项 PASS。
- 当前运行源基线为 `57cea75e0acbe3be8364a46b86b26f51b9a10437`，新候选只移植本项 Gate 改动，排除较新主线的 prepared-read 功能；`gate-continuity-runtime-source-proof.json` 绑定差异和实际 C/V/authdev 二进制。`gate-continuity-runtime-process-checks.json` PASS：全新隔离环境实际注册并取得会话后，正常重启、C/V 真进程 SIGKILL 于只读检查点更新中、证据缺失／过期后，同一令牌仍为 HTTP 200；临时不可用时为 503。两侧始终第 1 代，RECOVER 各 1 次（新空 Gate 初始化），FREEZE 各 0 次；测试故障触发器仅安装于新建一次性库。测试进程和数据库已停止，数据／证据保留。
- 扩展回归 `gate-continuity-full-tests.log` 未完成：在一次性库中已运行约 25 分钟；期间一个权限检查因打包缺少 `infra/prerelease` 失败，补齐后在 `gate-continuity-input-recheck.log` 单独复跑 PASS（真实 SQL + race）。用户催促进度后收敛验证范围并停止额外全量运行，保留原始 FAIL／interrupt，不宣称完整 PASS。仅停止本任务 Go 测试进程，runner 已清理其自建临时数据库。GitHub CI、Push／PR／合并均 NOT_RUN，完整远程产品 CI 仍待验。
- 产品实现本地提交 `9a25efeca4e2884cd0650caba5a920c6381e4f80`。用户“直接帮我解决”及已确认的恢复目标用于当前独立试用实例修复，未扩展到主线合并或正式发行。已于 2026-11-16 11:47（+08:00）更新当前试用后端，采用上述精确被测二进制，不合入其他主线功能。
- 当前实例升级备份 `/home/hnuhole/public-trial-access-01/runtime/backup/gate-continuity-20261116-4634208d0a884331919d10945a38a92e` 包含两套 custom dump、原材料／程序／被改源码；安装仅应用新增可空 UUID 列及 V 的该单列 GRANT，保留原 Goose 版本记录，后续 C43／V9 的幂等迁移可接管，不伪造已运行其他迁移。安装前后业务数据／序列摘要、密钥／证书／配置一致，Gate 始终 C OPEN:2 / V OPEN:4，累计 RECOVER 保持 2/4。原主线与其他 Worktree 未改动。
- `gate-continuity-installed.json`／`gate-continuity-live-runtime.json` 绑定运行补丁、备份和实际二进制；`gate-continuity-live-session-checks.json` PASS：更新前建立的既有任务合成会话仅在内存保留，更新后未再次登录，同一令牌访问会话／频道／首页均 200，实际 Frp C/V 严格 TLS 健康入口均 200；最终 Status PASS，后端／代理 active。原来已经因上一轮安全恢复失效的第 1 代会话不回退复活，本次升级没有新增撤销或恢复代次。
- 当前交付：代码完成；聚焦真实 SQL、进程与当前实例验证 PASS；完整远程 CI 待验；未 Push／PR／合并；本机试用修复已生效，未正式发布。测试仅使用自建隔离库及原任务合成账号，没有发送真实 OTP、操作真实用户账号、重置正式 schema 或发布新帖子。
- 2026-11-16 后续授权：用户明确“把这个修复推送上去，跑完 GitHub CI，通过后合并主线”，覆盖本任务分支 Push、创建 PR、修复适用 CI 并在当前 head 产品 CI 通过后合并 `origin/main`；不扩展为正式发布或其他任务合并。已核对最新 `origin/main` 仍为 `1d829b2`，无新增主线差异。

- PR #10 CI 兼容修复：性能对比的旧侧采用可达主线提交中与原批准 SHA256 完全一致的观测代码，不复制新版 Gate 产品逻辑。CI 期间主线 PR #7 合入占用 C43；已合入主线 d1a795d，将本项迁移顺延 C44（V9 不变），更新完整迁移链断言。前文 C43 指升级时历史编号，当前仓库正式迁移入口为 C44。当前试用的可空列不变，C44 仍可幂等接管。
