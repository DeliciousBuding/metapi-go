# BLOCKED

无

## 决策记录（无需裁决，仅留档）

- **失败恢复语义已存在，本 PR 为其加固并补测**：`applyBatch` 的聚合 upsert 与
  checkpoint 水位线推进本就在同一事务内（upsert 失败 → 整体回滚 → 水位线不动 →
  下次 pass 重投影同一 id 区间），与 octopus `restoreStatsDirty` 的「摘取脏集、
  失败放回」等价（此处脏集 = 水位线之上的 proxy_logs 行，无需内存回放）。
  因此未新增「内存待写集」，而是：① checkpoint UPDATE 0 行时整体失败回滚（防止
  提交聚合却不推进水位线 → 下次重投影双计）；② 失败日志带批次/水位线上下文；
  ③ 注释固化恢复语义；④ 触发器注入失败的单测（SQLite + PG 变体 + 多批次边界）。
- **失败返回 nil 未改**：`RunProjectionPass` 失败时返回 nil（与 in-flight 信号
  同形）是既有契约且无生产调用方，本次不改，失败由 checkpoint `last_error` 记录。
