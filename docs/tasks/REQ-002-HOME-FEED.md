# REQ-002 — 列表批量读取与增量分页

- 分支：`codex/perf/home-feed-batch`；[草稿 PR #11](https://github.com/zewbby/SpeechFUL-Develop/pull/11)。
- 授权：用户要求实施、推送及CI；本轮要求精简提交、仅展示最佳实测记录，并将当前接口性能门槛设为35%。未授权合并或部署。
- 状态：产品代码已实现；生产代码与e9一致。本轮仅精简诊断与记录、更新验收策略。当前提交检查见PR实时状态。

## 目标与接口

首页及频道首屏10条、刷新最多新增5条并保留旧内容、旧方向续页5条；12条新帖按最新5条、缺口5条、2条补齐。首页、频道、作者、我的帖子、点赞、收藏共用批量读取；其他列表页大小保留。协议及存储回滚边界见[列表契约](../design/feed-backend-contract.md)与[Gate契约](../design/auth-authorization-safety-gate.md#18-有界签名检查点存储)。

## 验收与验证

- 当前接口p95降幅门槛35%，所有有效轮次与路径仍逐项检查；最佳单项用于展示，不代替完整验收。
- SQL主体次数不随页大小增长，权限、会话／代次、fresh proof、最终授权事务及持久化检查保留。
- 保留分页、授权隔离、持久化损坏／失败及升级回滚回归，以及复核所需的最小性能测量器；删除临时阶段／尾延迟和Redis故障诊断。
- e9真实PG、配置、全Go构建与相关vet验证记录：`E:/Hnuhole-env/req002-home-feed-validation-20261116/sol-e9-integrated-review-01.json`。本轮35%策略对同一批原始数据离线复核，21项均满足；不是新增测量。
- 本轮聚焦验证：Python策略5项PASS；HTTP与commentsnapshot两个Go包仅编译PASS，格式检查PASS；未重跑真机或压测。
- 旧候选真机证据保留原SHA，不能冒充e9或后续集成的新整机实测；完整索引：`E:/Hnuhole-env/req002-device-20261116/evidence/r10-final-phone-evidence-index-01.json`。

## 最佳实测记录

**首页20条同页对照，第2轮：p95 7.845ms → 2.622ms，降低66.58%。** 这是同版本三轮中改善最大的单项，不是所有路径的平均值。

实测source `09491e6e170855ea0bb817d3e020ff9942afb5cc`，tree与产品e9相同；[原始CI数据](https://github.com/zewbby/SpeechFUL-Develop/actions/runs/43991485829/artifacts/14288337622)保持原样，ZIP SHA256 `945229c73e40bf9e410f5b6883354b7a8147182e566020da66688ec8953f131d`。完整复核记录：`E:/Hnuhole-env/req002-home-feed-validation-20261116/req002-policy-35-best-result.json`。

## 交付边界

此前REQ001独立性能CI失败仍单列，不被本轮35%策略改写。小米测试已结束、租约已释放；旧vivo离线的临时保持唤醒设置恢复仍待其可用。历史记录可由Git及既有CI追溯。未合并、未部署。
