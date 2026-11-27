# B3a 存储无关轮次模型

本包只处理有限ID/私有元数据、纯状态转换和合成测试，不读取数据库、账号数据、系统时钟、随机源、环境变量或网络，不启动采集。所有ID、UTC微秒时间、seed、策略、binding及存储codec密钥由调用方显式提供。B1选择器保持原字节，本包单向引用它。

入口：`New`/`BuildManifest`固定最多100个唯一canonical非nil UUID；`PageInput`返回当前固定10槽中未omit的ID；`Project`接收A内部Unavailable InputIndex，保持next原边界，首次下发后重试不续期；`Feedback`只接纳已下发页成员，消费bit防重复，按首次下发时间更新曝光截止；当前确认不可用的反馈成员也收缩omit。返回的Record包含轮次和账号历史两者，未来必须在原最终授权事务原子提交，并同时检查两份revision。

`Binding`/`Context`只是相等性和可信时间比较输入，不证明Gate、账号、会话、筛选或内容可见。后续适配器必须由A scope loader在同一最终业务事务复核当前筛选/revision，再按实际签名投影。候选Eligible、客户端曝光、HMAC存储签名都不能代替这一步；本包不保存/返回正文。

`ExperimentalPolicy`的30min轮次、10min反馈、5min下发及72h曝光窗口仅本地可配置实验值，没有启用开关；下发历史上限1024，曝光历史上限2048。过期物理行仍占模型容量，`PruneExpired`按最早到期跨两类历史最多移除128条；真实清理需先提交结果。`Project`/`Feedback`容量失败返回零新Record且原输入不变，不部分消费，不淘汰未过期曝光。

`Encode`/`Decode`是私有内部存储模型codec：固定版本、≤512KiB、显式32B测试key与keyID、域分离HMAC、canonical JSON、严格字段/结构/时间/掩码验证。它没有加密功能，不是正式cursor/intent/page receipt wire格式；生产密钥与持久化未接入。

`CandidateRepository`仅定义绑定当前事务的未来有界读边界，B3a没有PG实现。`ValidateCandidates`核对快照时间/seed/通道、应用原始行上限及400候选，B1继续负责特征校验；`BuildManifest`最多10次纯Select并传RoundSelected保证同轮唯一。Work统计是适配器断言，不是EXPLAIN/SQL实测。

`TupleValidator`用于比较后的单次Start提案：完整tuple（包括接受策略摘要）与服务端账号域共同定义identity，有限服务端时间窗和精确保留截止不可省。`Receipt.Resolve`只比较已提交结果。HTTP握手、唯一写入与跨进程原子性由服务端接线负责，详见[推荐交接](../../../../../docs/tasks/AB-final-acceptance.md)。

`Admit`只对调用方提供的测量数值、完整到期工作和名额预留构造保守调度证书。缺失/过期测量拒绝，安全吞吐最多取测量下界50%，每个job预留暂停时间；释放前的空闲能力不可挪作未来突发。它不认证测量或证明未来数据库吞吐；合成证书、内存CAS例子、race检查均不等于真实PG/容量通过。

`ReserveExpiryWindow`对尚未知晓的后页下发时间逐桶保守预留：每个可能事件时间桶按最多100行预算，release取桶尾+保留窗、deadline取桶首+10min。实际事件发生后再原子转换/释放多余预留；没有对应持久账时，这个helper不构成放量许可。`BuildManifest`还校验实际selector配置摘要与窗口，真实跨配置历史适配须尊重已存绝对Until。

现有 Feed backend CI 的全包测试、构建和 vet 覆盖本包。聚焦 race 检查见[推荐核心说明](../README.md)。
