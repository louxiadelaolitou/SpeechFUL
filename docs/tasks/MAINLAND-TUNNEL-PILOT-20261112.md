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
