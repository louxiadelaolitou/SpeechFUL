# REQ-005 — 帖子搜索查询加速

- 模式：常规；负责人／状态维护人：当前 Session。
- 分支／Worktree／PR：`codex/perf/post-search-documents`；`D:/SpeechFUL/.worktrees/req-005-post-search`；PR 未创建。基线 `47bf7e9c3231e3e45e078280086d7fc45025f4d9`（2026-11-16 已 fetch 并核对 origin/main）。
- 授权依据与边界：用户先要求 plan，随后确认性能硬门槛，再要求“拉分支开始做，视情况变化调整，不要死板一直按照计划”。授权本任务隔离实现、验证及必要本地提交；用户后续明确授权“推送分支并创建 PR，跑 CI，合并”；本次已授权推送／PR／CI／合并，未授权部署。需求附件的流程文字不另产生执行授权。
- 需求确认：保留中文及 Unicode 字面子串、ICU lower、相关性与最新排序、频道分组、过滤、全局 search_content_revision／旧游标失效、最终 C Gate 重验和一次重试；优化候选查询及续页成本。用户结构化回复“采用硬门槛（推荐）”：固定10万篇及相同负载，选择性较高的≥3字搜索 p95 降低≥50%；1～2字和高命中词分别报告，p95 均不劣化超过10%。基线阶段冻结词集、首屏／续页场景及重复轮次。无未决产品取舍；根据证据调整技术细节。
- 更新日期：2026-11-16。
- 状态：实现 已完成；验证 本地 PASS／远程产品CI NOT_RUN（待验）；合并 未合并；发布 未发布。

## 目标与范围

建立当前公开内容搜索读模型、同事务同步、可恢复回填、pg_trgm 候选索引及查询开关，交付新旧逐页差异检查、并发故障验证与性能报告。保留业务表最终授权，沿用现有搜索界面、历史及精确标签页契约。回退至旧查询后仍维护读模型。

最终查询路由：开关关闭／定点postId查询保留原SQL；开关开启且读模型通过校验时，选择性词使用UTF-8 hex候选键GIN＋逐字段／逐标签strpos复核，高命中、短词及不可提取词使用规范化文档全量精确扫描。后者不使用GIN，不改变最短词长或匹配含义；排序／频道／过滤／游标和最终C事务沿用原逻辑。

## 接口与依赖

- 需求001已由 PR #3 合入本基线；搜索本身已有 Gate 外 prepare 与最终事务批量投影，不重做授权基座。
- 主线包含0045标签post_id索引及0046同步跳过优化；搜索 lower 使用 `und-x-icu`，不复用使用 `C` lower 的标签索引键。
- 新路径显式开关默认关闭；完成一致性校验才允许启用。迁移、运行时最小权限、历史schema及正式 Goose 升级共同验证。
- 验证仅使用任务自建 WSL2 Linux 文件系统上的一次性 PostgreSQL；不得连接正式／日常业务库。

## 验收标准

- 新旧结果逐页一致，覆盖中文1／2／3字、中英混合、Unicode大小写、标点、`%`／`_`／反斜杠、标签边界、无命中／高命中、编辑后当前内容，两种排序、全频道／单频道及 lookahead。
- 发布／编辑／删除／审核／内容擦除与回填交错无漏更新；最终重验拒绝旧内容、撤权与版本冲突；旧游标按原规则拒绝。
- 100,000篇、冻结的相同数据与负载下，选择性较高的≥3字 p95 降低≥50%；短词与高命中词 p95 各不劣化超过10%。单列首屏／续页、SQL扫描量、索引体积及写入额外耗时。
- Go格式／构建／test／vet、真实SQL／迁移／最小权限／HTTPS和适用产品CI证据绑定最终代码；缺项如实待验。

## 当前进度与证据

技术调整（固定负载未变）：首轮诊断显示两字词及高命中词退化。实际SQL探针证实原夹具的数据库 `LC_CTYPE=C` 下，直接中文文本的 `show_trgm` 为空，给索引指定ICU也不能解决；10,000行小样本中文索引重验10,000行。改用ICU lower之后的UTF-8 hex ASCII候选键，探针同词只访问10行，仍在排名／LIMIT前用逐字段／逐标签strpos精确复核。短词／不可提取词回退原查询，候选事务局部force_custom_plan避免复用稀有词的通用计划。新增ICU实际版本标记，升级后不允许使用未重建的规范化数据。首轮3样本诊断不是验收PASS，保留在外部 `perf-diagnostic-v1.json`；正式负载、门槛和三轮规则未变。

第二轮诊断 `perf-diagnostic-v2.json`：三类选择性词端到端降至约27–54ms，中文实际GIN候选100行；高命中词仍慢，EXPLAIN显示优化器将实际100,000行估为10行，导致100,000次posts主键查找。新增最多128行的SYSTEM页采样（只读、Gate外）选择执行路径，样本至少32行且命中≥25%时走完整原查询；它不限制或抽样搜索结果，也不作为授权依据。短词直接原查询。此项处理宽编码键缺少有效列统计的实际问题，性能负载／阈值保持v1。

首次正式三轮（产品`ae4ad28`）FAIL：36项中高命中首屏第1轮602.19→667.83ms（1.109倍），超过1.10；其余35项达标，不能替代整体通过。readtiming显示该轮候选查询574.12→586.43ms，路由0.43ms，anchor_write尾延迟36.88→132.98ms；主要差异在anchor I/O，未修改Gate或放宽验收。保留完整`performance-ae4ad28-fail.json/log`。为增加真实查询余量，将高命中路径改为无LIKE／GIN的全量规范化文档精确扫描，减少跨表拼装，仍按原完整候选排名及分页；低频索引和短词回退不变，重新绑定版本验收。

第二次正式三轮（产品`1213a5d`）FAIL：高命中规范化扫描各已测轮次改善约22%～28%，但二字词续页第3轮533.12→621.39ms（1.166倍）超线；完整JSON／日志保留。短词仍执行原SQL，改为不走GIN的规范化文档精确扫描，直接减少跨表读取和逐行lower；原始查询开关回退、词长及匹配语义均不变。性能门槛／数据／轮次不变，不通过重复同版本挑选通过轮次。

第三次正式三轮（产品`ec2a626b69c32f381b0e41560db9a920c3c7a0b7`）PASS：36项全部达标，1440个测量样本保留在[性能证据](evidence/post-search-performance-20261116.json)。从各组20个原始样本独立重算p95、比例及36个场景键，均与报告一致；测试耗时940.50s。正式负载及门槛与前两次相同，只调整有实际退化证据的查询路径。

扩大schema回归发现并修正：`TestPostSchemaPartyInventoryAndPrivateColumnsPostgres`禁止c_posts内视图；统一源定义改为owner-only规范化函数（点查与全量校验使用独立常量SQL形状），不放宽既有无视图约束。函数同样校验body摘要并撤销runtime执行权；失败日志`backend-full-v1-fail.log`保留。之前只针对源视图的证据不冒充此修订通过。

并发复核：回填按keyset处理且不等待已被业务占用的帖子行，遇忙停止在该行之前，首次即忙返回-1；运维脚本有界重试，提交游标保持原位。避免与账号清理等多帖事务的不同锁序形成循环等待，也不因SKIP LOCKED跳过历史缺行。同步函数仍在同一帖子行围栏后重读当前源数据；编辑、删除与回填不允许旧批次覆盖新状态。

性能口径 v1（首次大数据测量前冻结）：100,000篇当前公开帖，固定 `req005.post/task.<n>` ID 和 SQL种子；每篇普通正文重复8次，中文标题稀有词每997篇、ASCII正文词每701篇、中英混合标签词每571篇。词集依次为 `稀有词`、`needle`、`Lab研究`、`学`、`校园`、`校园生活`；前三项按50%门槛，后两类短词及高命中词按110%上限。全频道 relevance，页大小20，首屏及第二页分别报告；同库同配置、单并发、12连接上限、每轮每路径预热3次与测量20次，旧／新成对交替顺序，固定3轮且每轮分别判定，禁止挑轮次。两种排序／频道范围的全部组合由等价性回归覆盖。`POST_SEARCH_PERF_QUICK=1` 仅跑1轮／3样本诊断，绝不计为硬门槛验收。端到端样本走原生C Gate（测试可信时钟固定以避免长测量过期），同时记录 readtiming 阶段及实际生产SQL的 EXPLAIN ANALYZE BUFFERS、索引体积、1000行回填批次耗时和事务回滚的写语句额外成本。

| 项目 | 被测版本／环境 | 结果与证据 |
| --- | --- | --- |
| 入口核对 | `47bf7e9`；D盘源码库／origin SpeechFUL-Develop | PASS：隔离分支已建立，原 main 与其他 Worktree 均保留 |
| 实现 | `ec2a626b69c32f381b0e41560db9a920c3c7a0b7` | PASS：读模型、事务同步、可恢复回填、索引与路由、独立开关、schema／权限完整性检查及性能workflow已接入 |
| Go基础检查 | 同产品版本；Go1.22.2／WSL2；`go test -mod=readonly ./...`、`go build -mod=readonly ./...`、`go vet -mod=readonly ./...`、回填脚本`sh -n` | PASS；任务独占Linux目录 `host-go-test.log`／`go-build.log`／`go-vet.log`。无DSN的host测试不证明SQL |
| 聚焦真实SQL＋race | 同产品版本；PostgreSQL16.15／Goose3.22.1；任务独占Unix socket、一次性C/V库；命令如下 | PASS：7种Goose升级、等价性、注销／隐藏／删除／筛选／Gate换代／游标与重试、占用行回填断点、插入式发布、源函数摘要／波动性、触发器／最小权限、无视图及私有对象清单；`short-scan-race-pass.log` |
| 扩大后端回归＋race | 同产品版本；同一隔离环境；`FEED_TEST_RACE=1 FEED_TEST_TIMEOUT=20m`，取消`FEED_TEST_PATTERN`后执行`sh services/api/authlab/run-feed-isolated.sh`默认选集 | PASS：98个顶层测试、0个FAIL；authprivacy／HTTP／runtime／rankings四包命令均成功。9个SKIP：性能1项已另行正式PASS，其余为专用评论验证器、Redis及分享／标签／排行榜Dart探针前置条件缺失，不代表相关场景验收通过；`backend-full-ec2a626-pass.log` |
| 运维回填脚本 | 同一一次性库，实际执行`backfill-post-search.sh 1` | PASS：独立提交回填、校验及VACUUM／ANALYZE；补验修复了环境PGDATABASE不展开URI的问题，现显式传入dbname |
| API／搜索客户端 | Flutter／规范源码与基线一致；Linux Flutter3.47.5、原锁离线依赖；搜索／API两文件；当前Dart探针＋`ec2a626`服务端真实C HTTPS／SQL | PASS：13份OpenAPI、29受保护操作静态检查；搜索analyze无问题、65项Flutter测试；最终服务端版本的Dart探针完整搜索链路通过；`openapi.log`／`mobile.log`／`dart-ec2a626-pass.log` |
| 邻接客户端扩大检查 | 同基线、Flutter及测试源码无差异；额外加入标签／编辑UI文件 | PASS：12项标签／编辑测试全过。原8项MediaComposerFixture超时定位为系统只有libsqlite3.so.0，测试原生资产要求libsqlite3.so；仅在任务私有lib目录建立别名并设置本次LD_LIBRARY_PATH，未改Flutter／夹具或系统库。`mobile-adjacent.log`；保留原失败日志 |
| 性能硬门槛 | 同产品版本；固定口径v1；`POST_SEARCH_PERF=1 FEED_TEST_RACE=0 FEED_TEST_PATTERN='^TestFeedPostSearchDocumentsPerformancePostgres$' sh services/api/authlab/run-feed-isolated.sh` | PASS：36/36项、1440个测量样本；`performance-ec2a626-pass.log`与下述可移植JSON；四次quick均仅诊断，两个旧产品版本正式FAIL均保留 |
| 远程产品CI／独立审查 | 未推送，未创建PR；性能workflow已添加 | NOT_RUN；本地检查不代表远程CI或独立成员批准 |

聚焦SQL命令（先将Go／PostgreSQL加入PATH，并设置`FEED_GOOSE`为Goose3.22.1）：

```sh
FEED_TEST_RACE=1 \
FEED_TEST_PATTERN='^(TestPostSearchDocument|TestFeedPostSearchDocuments|TestFeedGooseFreshAndUpgrade|TestRealHTTPSPostSearchDocuments|TestPostSchemaPartyInventoryAndPrivateColumns)' \
sh services/api/authlab/run-feed-isolated.sh
```

## 最终性能结果

下表为三轮各自p95的范围（ms），改善取三轮中最小值，每轮分别验收；完整逐轮样本、阶段耗时及生产SQL计划在[性能证据](evidence/post-search-performance-20261116.json)。计时范围是原生Go SearchPosts到最终C Gate及投影完成，不包含HTTP传输、移动设备绘制或生产负载。

| 词集／场景 | 原查询p95 | 新查询p95 | 最小改善 | 结果 |
| --- | --- | --- | --- | --- |
| 稀有中文／首屏 | 893.95–1173.49 | 54.46–63.27 | 93.32% | PASS |
| 稀有中文／续页 | 855.84–948.15 | 44.58–70.29 | 92.33% | PASS |
| ASCII／首屏 | 780.26–854.19 | 42.45–49.94 | 94.15% | PASS |
| ASCII／续页 | 754.35–1065.48 | 40.27–59.75 | 93.38% | PASS |
| 混合标签／首屏 | 842.82–886.07 | 52.47–65.40 | 92.54% | PASS |
| 混合标签／续页 | 828.55–1092.18 | 43.91–64.51 | 94.09% | PASS |
| 一字词／首屏 | 837.10–927.22 | 405.52–448.33 | 47.12% | PASS |
| 一字词／续页 | 792.67–1081.72 | 384.39–526.36 | 48.75% | PASS |
| 二字词／首屏 | 611.11–621.50 | 490.95–502.32 | 19.18% | PASS |
| 二字词／续页 | 535.89–636.08 | 459.42–471.19 | 12.07% | PASS |
| 高命中／首屏 | 604.11–618.49 | 481.18–544.62 | 11.94% | PASS |
| 高命中／续页 | 528.45–568.81 | 414.10–453.41 | 20.29% | PASS |

实际首屏计划：旧路径读取10万条posts、publication_tasks和attempt_contents；三类选择性新路径的GIN候选分别100／142／175行，随后只读取相应数量的posts并完成原精确过滤。短词和高命中仍扫描10万行规范化文档及posts，省去历史任务／正文拼装和逐行lower；不声称此类路径已消除全量扫描。

GIN索引40,026,112字节（38.17MiB），读模型总计248,111,104字节（236.62MiB，含索引／TOAST）。10万行回填52.47s，1000行批次最大1.09s。20次回滚事务中的编辑语句p95为1.096→1.127ms，额外约0.032ms／2.88%；此项隔离的是同步触发器语句成本，不代表持久提交或高并发写入吞吐。

外部原始证据根目录：Windows `E:/Hnuhole-env/req005-post-search-20261116`，Linux `/home/hnuhole/req005-post-search-20261116`；测试源码为产品提交的Git archive，数据库／证书由任务夹具独占创建和清理。可移植JSON绑定产品提交；后续仅任务记录／证据提交不改变被测产品内容。

## 阻碍与下一步

无产品取舍阻碍。本地产品实现、聚焦及扩大回归、最终版本Dart→真实HTTPS链路和正式性能硬门槛已完成。远程产品CI及独立审查NOT_RUN，整体保留待验；推送及PR获授权后执行送审，合并及部署另待明确授权。未进行设备／iOS／生产验收，本任务未改移动端产品代码，也未恢复历史延期项目。

运维顺序（尚未执行部署）：先保持 `posts.searchDocuments=false` 应用0049，使用迁移owner明确设置 `POST_SEARCH_BACKFILL_DSN` 执行 `sh services/api/authlab/backfill-post-search.sh 1000`；每批独立提交，断点在 `post_search_document_state.last_post_id`，失败可重跑。双向全量一致性校验、VACUUM／ANALYZE完成后才显式开启 `posts.searchDocuments=true`。回退仅关闭此开关，保留表和触发器持续维护；需要重新全量修复时owner先关开关，将ready置false、last_post_id复位，再回填并校验。运行时只有读权限，不能自行标记ready或执行维护函数；缺结构、索引、同步hook、源／维护函数完整性或ready时拒绝开启。

## 主线整合与送审

2026-11-16：整合最新主线`bf5c0ae`（私信目录及评论优化已合入）。主线占用0047／0048，搜索迁移顺延为0049；三个迁移测试的最终账本统一49，保留搜索回填与schema检查。查询算法、触发器／维护函数正文及性能门槛未改，原`ec2a626`性能证据保留原版本范围；整合版本由PR CI重新验收。
