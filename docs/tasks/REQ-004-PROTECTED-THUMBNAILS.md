# REQ-004 — 受保护图片缩略图复用

- 模式：常规；负责人／状态维护人：当前 Session；用户指定Astra XHigh后使用其独立临时Worktree定位修复，权威记录仍由当前Session维护。
- 分支／Worktree：`codex/perf/protected-thumbnails`；`D:/SpeechFUL/.worktrees/req-004-protected-thumbnails`；基线 `1d829b2623a6c32830ee1b74fae6c99a436ad3b3`（开始时与远端 main 一致）；PR [#7](https://github.com/zewbby/SpeechFUL-Develop/pull/7)（draft）。
- 授权依据：2026-11-16 用户要求“按这个计划开始实施需求004”，覆盖本地实现、隔离验证及必要本地提交；用户随后要求“为什么，现在可以验收”，覆盖继续验收所需的任务分支推送、草稿PR、CI跟进与已连接手机隔离测试；合并与正式部署仍未获授权。附件方案中的操作文字不产生额外授权。
- 需求确认：用户已确认旧图片按首次访问生成，另提供手动限速／可停止回填；重复缩略请求 p95 至少下降60%、真实首页首屏可见图片全部显示时间至少下降30%，均为硬门槛。首屏测量固定设备、网络和内容，服务端已有缩略图、手机无缓存；首次生成耗时另测。新帖上限3图，历史4–9图只做兼容，不采用附件“九图首屏”。本次没有未决产品取舍。
- 更新日期：2026-11-16。
- 产品版本：初轮实现`9754803`；恢复兼容性修复`70f7363`/`d0db7cd`，测试TLS`8eb08b4`；客户端性能修复`67ad436`。各项证据仅适用于对应版本和明确差异。
- 状态：实现及Astra XHigh修复完成；`d0db7cd`、`8eb08b4` 各14项workflow全部PASS；客户端修复后的当前head CI待核对。服务端60%门槛PASS；手机同客户端开关归因未达30%，全部失败保留。用户已确认正式30%按需求前后完整版本比较，随后接管真机，正式双版本验收待用户执行。未合并、未发布。

## 目标与范围

复用私有 C PostgreSQL 内的缩略字节，减少原图读取与受控 Python 转码；保持每个请求的 Bearer／会话代次／最终 C Safety Gate／当前源图选择和可见性验证。仅帖子 thumbnail-v1；640px静态首帧、512KiB上限、原图、上传、发布完整性、租约、保存许可和 no-store 契约不变。不扩展头像、评论、私信缩略图或客户端长期缓存。

## 接口与依赖

- 需求001已随 PR#3 合入基线，复用其独立只读准备事务、锁外 CPU 工作、有界最终重验和最多一次冲突重建。
- 衍生记录绑定帖子＋内容版本＋规格＋编码版本，并验证完整源元数据／摘要，覆盖原发帖与编辑图片。生成在 SQL／Gate 外；首次持久化在最终授权事务内，和源删除／失效同事务串行，不在准备阶段新增业务写入。
- 同实例合并昂贵准备，所有等待者仍各自最终授权；跨实例唯一键仅保证单份落库，不承诺全局只转码一次。取消、并发和待处理内存均需有界。
- 可单独关闭复用；数据库生命周期触发器始终清除已写衍生字节。

## 验收标准

- 连续命中原图读取0、Python启动0；20并发同键最多同时转换一次，不跨请求复用授权结果。
- EXIF／动画首帧／尺寸／格式／版本隔离／原图保存字节回归；生成及命中期间删除、编辑、隐藏、拉黑、撤销会话、冻结均不放行失效字节。
- 真实一次性 PostgreSQL 验证约束、受限角色、删除竞态、容量预算与前向迁移；实际 HTTP 和最终提交失败／取消／ACK丢失回归。
- 固定基线与负载验证服务端60%和设备首屏30%硬门槛；冷生成单列。缺真机／CI不报整体PASS。

## 初轮本地进度与证据（产品提交9754803）

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 基线核对 | main `1d829b2` | PASS：实际仓库D:/SpeechFUL、远端SpeechFUL-Develop，原工作区干净；隔离任务分支已创建。 |
| 实现 | 基线＋本任务提交的产品差异 | PASS：0043私有衍生表／payload预算账本、原图及编辑图来源绑定、版本化真实Python转码、锁外singleflight、逐请求最终授权内首次持久化、源同事务擦除、默认关闭复用、分页维护、聚合监测、可限速停止的HTTPS预热工具及媒体契约。 |
| 后端与契约 | WSL Linux；Go1.22.2、PostgreSQL16、Pillow12.3.0；一次性私有socket数据库 | PASS：`go build -mod=readonly ./...`、`go vet -mod=readonly ./...`、`go test -mod=readonly ./...`；host SQL项SKIP不计真实SQL。固定依赖OpenAPI全规范检查、社区文字契约、预热边界2项PASS。 |
| 真实SQL／HTTPS／受限角色 | `functional-final.log`＋`schema-final.log`＋`inventory-final.log`，均含-race；最终有效产品文件与Linux被测副本逐字节核对 | PASS（组合分项证据）：命中零原图读取／零转码、20并发合并、两实例单份落库与账本、取消不影响其他等待者、原／编辑图片及ID重用、冷／热期间删除／隐藏／拉黑／会话换代／冻结、原图字节、COMMIT回执丢失、编码panic、容量满及分页孤儿清理、回退仍擦除、启动缺钩子／no-op约束／错误索引／账本漂移拒绝。初次表／特权函数清单FAIL由最终inventory聚焦PASS关闭，原始失败保留；不是声称早期整包都PASS。 |
| 正式前向迁移 | Goose v3.22.1；`feed-migrations.log`、`dm-migrations.log`、`avatar-migrations.log` | PASS：首页七条fresh／升级路径、DM及头像各两条，均到43，原资料不变、重复up及Down拒绝。保留常规路径最小schema39，不因默认关闭复用提高原配置要求。 |
| 重复缩略HTTP请求p95 | `target-7.log`中的 `TestThumbnailReuseHTTPSPerformancePostgres`；实际HTTPS＋原生C Gate＋真实Python；1024×768 JPEG，种子424242 | PASS：每路每轮5次预热＋40次测量，三组配对次序交替。基线／复用p95分别147.540／15.670ms、150.286／16.652ms、154.442／15.601ms，下降89.38%／88.92%／89.90%，逐组超过60%。每个请求检查字节不变和读钟区间有效；两路均开启001准备读取，仅004复用开关不同。 |
| 辅助服务方法性能 | `target-7.log`中的 `TestThumbnailReusePerformancePostgres` | FAIL（环境测量无效，非达标证据）：RAW138,688,290ns，Go区间141,019,237～141,020,118ns，读钟不确定性881ns，超过既定1%／1ms保护，立即停止该组。早期服务方法三组约90%只作诊断历史，不代替最终HTTPS结果。未改系统时钟、校正耗时、放宽保护或删除失败。 |
| 设备首屏30%／产品CI | ADB无连接设备；本任务尚未推送／创建PR | BLOCKED／NOT_RUN：不能用HTTPS或组件测试冒充实际首屏绘制。已将复用回归、受限角色和三组性能纳入Post images产品CI；远端CI仍待本任务推送／草稿PR授权。 |

全部原始日志在 `/home/hnuhole/req004-protected-thumbnails-20261116/evidence/`；[脱敏摘要](E:/Hnuhole-env/req004-protected-thumbnails-20261116/evidence/summary.json)。运行者只创建／清理自己的临时数据库，未修改正式数据、既有服务、设备或系统时钟。最终HTTPS测量之后的产品变更仅涉及后台有界分页清理及其启动检查，已聚焦SQL复验；请求读取／生成／最终授权和HTTP响应逻辑相同，当前完整CI仍需绑定最终head。

## 阻碍与下一步

PR#7保持draft；客户端修复后当前head CI待核对。按用户最新指令停止本任务真机操作、释放设备占用，正式首屏30%双版本验收待用户执行，不以旧版与新版不同时段结果拼接PASS。任务收尾后通知会话“02”设备占用已释放，供其继续自身验收；不将REQ-002与REQ-004的门槛混用。未获合并或正式发布授权。

## 2026-11-16 继续验收

- 已检测到 vivo V2323A（10CEAG17RY003M7），既有设备租约已释放；沿用隔离测试包，不覆盖publictrial用户App。当前准备真实FeedScreen、原生vault、受保护TLS及C Gate的一次性数据测量；不是将HTTP耗时代替首屏。
- 已推送cd60f40并创建draft PR#7。该head的12项workflow中11项成功；Android prerelease的实际候选构建成功，rehearsal在备份恢复后启动失败。其根因是`pg_restore --no-privileges`使新触发器函数恢复PUBLIC EXECUTE，启动检查正确拒绝。补全canonical grant中的函数REVOKE及角色审计，新增真实dump/restore回归；原始失败run43791157808保留。
- 本次增量以cd60f40为基线：恢复权限修复＋独立手机测量工具；后台请求编码与最终授权逻辑不变。手机指标、最终head CI及恢复复验均进行中，不报整体PASS。

- 恢复复验进一步定位了PG16将等价AND括号扁平化的问题：仅byte_length三项AND的括号变化使整体CHECK指纹改变。现在严格接受已核对的fresh／restore两种完整指纹，不移除任何长度／摘要／尺寸约束；真实dump/restore＋canonical ACL＋受限runtime启动检查已PASS。第二轮恢复失败run43792697067保留，等待当前修复的CI。
- 本机Linux NDK clang为空，native构建无产物；停止使用该工具链，改用现成Windows编译器。不恢复整套SDK、不改依赖锁或生产配置，手机指标尚未完成。


## 手机性能定位（用户指定Astra XHigh）

- `d0db7cd` 的14项workflow全部PASS，包含 [Post images](https://github.com/zewbby/SpeechFUL-Develop/actions/runs/43795625667)、[Android prerelease恢复演练及候选](https://github.com/zewbby/SpeechFUL-Develop/actions/runs/43795625872)。`8eb08b4` 仅调整手机测试TLS证书，其14项workflow也全部PASS。iOS项只证明构建，不声明iOS设备通过。
- `d0db7cd` 云端实际HTTPS三组 p95：146.359/9.153ms、145.572/9.020ms、146.672/9.088ms，分别下降93.75%/93.80%/93.80%，60%硬门槛PASS；辅助服务方法三组均下降约95%，冷生成156.909ms。真实dump/restore完整性回归PASS。此前WSL时钟异常仍保留原FAIL，云端结果不是覆盖删除原结果。
- 手机：vivo V2323A、Android15、现有1080×2400覆盖分辨率/480dpi，USB reverse固定本地网络；真实FeedScreen中一个合成帖子、一张1024×768 JPEG（630923bytes），不是九图。每路2轮预热＋6轮配对测量、交替次序；每轮清Flutter图片缓存并新建媒体仓库，服务端已有衍生图，真实Bearer/原生vault/TLS/C Gate。偶数样本取上中位数。
- DEBUG首轮基线1626.712ms，复用1540.033ms，下降5.33%，FAIL；AOT profile首轮基线1530.038ms，复用1455.081ms，下降4.90%，FAIL。AOT帖子出现约489–636ms、图片清单返回约710–1113ms，JIT不是主要原因。原始样本分别 `E:/Hnuhole-env/req004-protected-thumbnails-20261116/evidence/device-debug-run1.json`、`device-profile-run1.json`；不放松30%门槛，不删除失败。
- 用户随后明确要求“用Astra XHigh来找原因修复”，本次按该指令调用Astra（XHigh）定位，使用独立临时分支/Worktree `codex/perf/req004-metadata-noop` / `D:/SpeechFUL/.worktrees/req004-metadata-noop` 调查会话元数据的冗余原生持久化；主Session负责手机、测量和集成，权威记录仍为本文件。
- 测试fixture改用仓库已有mobileLabCertificate并按现有设备fixture每分钟签发真实时间的fresh安全证据，不修改系统时钟、不跳过Gate。此前fixture空等构建导致证据5分钟过期返回503，属于未进入测量，不计首屏样本。AOT采用显式隔离包的profile构建，run-as只传输其私有合成凭据；未覆盖publictrial。

- Astra第一处修复：仅会话元数据调用选择`skipUnchanged`；每次在AuthStore串行队列内仍完整原生读取/解析，只有待写JSON与本次原始记录字节完全相同才省去重复write/readback。真实期限变化仍原子写盘＋回读，普通写入、空记录、非规范记录不跳。新增撤销/换账号、损坏/读失败、真实写/回读失败、异步authority失效、较旧响应及普通写边界测试。
- 该修复AOT同客户端两路配对结果：基线1020.577ms、复用851.051ms，下降16.61%，仍FAIL；每路每轮原生读取6次、写入0次，读取合计约116–202ms。原始`device-profile-noop-run1.json`保留。整体首页虽明显改善，不能将旧客户端额外开销计入纯缩略复用收益；继续定位固定开销。
- 聚焦安全回归首次35项PASS；扩展9套205项PASS/1项旧故障注入未触发（相同期限不再写盘），保留原日志。该fixture改为返回真实更晚期限，并保留/加强“写失败则无设备数据、清空会话缓存”断言，4项聚焦PASS；最终当前head CI仍需完整核对。

## 修复与验收交接

- `67ad436`另将帖子GET改为复用TLS连接；每个请求仍使用本次Bearer、验证no-store及完整响应并走当前C Gate。POST/PUT/DELETE命令继续关闭连接，不添加重试。真实loopback TLS6项＋原post_api43项共49项PASS，覆盖同socket不同Bearer/响应、401、截断体丢弃连接、三种命令接收后断开各仅发送一次。
- 最终客户端同开关三批AOT归因全部保留：858.192/657.934ms（23.33%）、815.483/701.673ms（13.96%）、896.075/789.023ms（11.95%），均FAIL。原始`device-profile-keepalive-run1.json`至`run3.json`。固定1帖1图和计时起点不变；同机另有独立任务Go/SQL背景负载，未停止他人进程。
- 用户随后确认正式首屏30%基准为“需求004修改前完整版本 vs 包含本次修复的完整版本”；上述同客户端开关结果仅作独立归因，不删除、不改判。需要固定双版本、相同内容/网络/设备、服务端预热/客户端无缓存、顺序交替重新测量；旧AOT约1530ms与新包不同时段约658–789ms仅作诊断，不能拼接正式PASS。
- 本地完整mobile回归`mobile-full-final.log`：1308 PASS、4 SKIP、1 FAIL；唯一失败为旧fixture用完全相同期限注入写故障，现改为真实更晚期限，保留/加强撤权与故障已消耗断言；`mobile-feed-write-final.log`聚焦31项PASS关闭该失败。`mobile-analyze-final.log`完整analyze PASS。证据绑定`67ad436`＋随后无产品变化的冗余import清理及该fixture修正。未将整包原始失败覆盖为绿。
- `67ad436`的CI分析发现新增TLS测试多余import，`1a630cc`已修正；最终当前head CI仍须核对，不能沿用`d0db7cd`全绿替代新增客户端实现。
- 用户最新要求后续真机由其测试，并在本任务完成后给会话“02”发送设备可用信息。已停止手机操作、正常结束root4临时后端（runner PASS退出并清理自己的PG）、释放`E:/Hnuhole-env/device-leases/10CEAG17RY003M7.json`；收尾时ADB已无设备，未能确认卸载`.thumbnails`及删除其12个reverse映射，接手者重新连接后核对。未修改publictrial用户App。
- 原始baseline `1d829b2`临时Worktree `D:/SpeechFUL/.worktrees/req004-before-acceptance`及2个test-only fixture、archive/runner草稿保留供后续参考；NOT_RUN，未编译/启动，未创建baseline PG或服务，不称旧版后端已可用。正式验收须先核对这些适配文件。
