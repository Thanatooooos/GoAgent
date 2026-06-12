# Retrieve 评估报告

## 基本信息

| 字段 | 值 |
|------|-----|
| 评估日期 | YYYY-MM-DD |
| 代码版本/commit | |
| 配置摘要 | configs/application.yaml 关键项 |
| 样本文件 | testdata/retrieve_eval_samples.json |
| 样本总数 | |
| 评估模式 | offline / execute |
| Search mode | hybrid / semantic / keyword |

## Overall 指标

| 指标 | K=1 | K=3 | K=5 |
|------|-----|-----|-----|
| Hit Rate | | | |
| Avg Recall | | | |
| Avg NDCG | | | |

**MRR**:

## ByTag 指标

| Tag | Sample Count | Hit@1 | Hit@3 | Hit@5 | MRR |
|-----|--------------|-------|-------|-------|-----|
| alias | | | | | |
| diagnosis | | | | | |
| metadata | | | | | |
| coreference | | | | | |
| multi_condition | | | | | |
| keyword | | | | | |
| semantic | | | | | |

## 退化样本列表

| 样本名 | Query | 期望 | 实际 First Rank | 说明 |
|--------|-------|------|-----------------|------|
| | | | | |

## 结论

- 整体是否提升/退化：
- 主要受益标签：
- 主要退化标签：

## 下一步动作

- [ ] 调整 rewrite 术语规则
- [ ] 调整 search mode 默认策略
- [ ] 补充退化样本到评测集
- [ ] 针对某 tag 做专项优化
