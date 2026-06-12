# Query Rewrite 术语治理

## 维护位置

术语规则配置在 `configs/application.yaml`：

```yaml
rag:
  query-rewrite:
    term-normalization:
      enabled: true
      rules:
        - canonical: PostgreSQL
          category: component
          version: 1
          enabled: true
          aliases:
            - postgres
            - pg
```

代码入口：

- `internal/app/rag/core/rewrite/term_normalizer.go`
- `internal/bootstrap/rag/runtime.go` 中的 `buildTermNormalizationRules`

## 字段说明

| 字段 | 说明 |
|------|------|
| `canonical` | 归一化后的标准术语，必填 |
| `aliases` | 可命中的别名列表，必填 |
| `category` | 术语分类，如 `component`、`product`，可选 |
| `version` | 规则版本号，便于观测和回滚，可选 |
| `enabled` | 是否启用；省略时默认启用 |

## 发布流程

1. 修改 `configs/application.yaml` 中的规则。
2. 运行测试：`go test ./internal/app/rag/core/rewrite -count=1`
3. 部署配置并观察 rewrite trace metadata。

## 回滚方式

- 将单条规则 `enabled: false`。
- 或恢复旧版 YAML 配置。

## 命中观测

术语归一化命中信息写入 `rewrite.Result.Metadata.termNormalization`：

```json
{
  "changed": true,
  "matches": [
    {
      "alias": "pg",
      "canonical": "PostgreSQL",
      "category": "component",
      "version": 1
    }
  ]
}
```

trace 层可从 rewrite stage 的 metadata 读取该字段。

## 边界

- 术语归一化只做确定性替换，不新增语义。
- 不把术语词典写死在 Go 代码里。
- 不把术语归一化逻辑放进 LLM prompt。
