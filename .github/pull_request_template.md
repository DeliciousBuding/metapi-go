## 改动摘要

<!-- 一句话说明这个 PR 做了什么、为什么（不要写"改了什么文件"流水账） -->

## 类型

- [ ] feat（新功能）
- [ ] fix（bug 修复）
- [ ] perf（性能优化）
- [ ] refactor（重构，行为不变）
- [ ] docs / chore（文档或工程维护）

## 测试验证

<!-- 本地门禁：改动涉及前后端时勾选对应项 -->

- [ ] `go vet ./...` 通过
- [ ] `go test ./... -count=1 -race` 通过
- [ ] `cd web && bun run typecheck` 通过
- [ ] `cd web && bun run lint` 0 error
- [ ] `cd web && bun run test` 全绿
- [ ] 涉及 API/DB 契约时 `test-pg`（CI）通过

## 自查清单

- [ ] 无密钥/内部路径泄漏（gitleaks 会在 CI 检查）
- [ ] 用户可见文案已走 i18n（`t()`）
- [ ] 行为变更已补充/更新测试
- [ ] 需要时更新了 `CHANGELOG.md` / `docs/`（STATE/MASTER/log）

---

> 合并方式：Squash merge（master 受保护，禁止直接 push）。合并时请把 PR 标题改成符合 Conventional Commits 的提交信息，如 `fix(web): ...` / `feat(store): ...`。
