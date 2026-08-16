# BLOCKED

无

## 决策记录（无需裁决，仅留档）

- **衰减因子取 `failCount / 2`（整数除法）**：按任务要求「衰减而非清零」。
  failCount=1 时一次成功即归零（单次偶发失败不留记忆），failCount≥2 按几何
  衰减保留少量记忆。OAuth route-unit member 路径同步衰减（与 channel 路径一致）。
- **成功重置 4 字段不变**：cooldownUntil / lastFailAt / consecutiveFailCount /
  cooldownLevel 保持清零，仅新增 failCount 衰减写入。
- **`RecordProbeSuccess` 不动**：任务范围仅限 `RecordSuccess`（用户流量路径）；
  探针成功路径保持原语义。
- **gofmt**：`router_records.go` 在 HEAD 即非 gofmt-clean（map 字面量对齐风格），
  CI（golangci-lint 配置）未启用 gofmt/gofumpt，故保持文件既有风格，未重排无关行。
- **`web/dist`**：worktree 不含 gitignored 的 SPA 预构建产物，从主工作区拷贝同源
  dist（本 PR 仅改 Go 代码，前端内容与 master 一致）以便本地 `go build`/pre-push 通过。
