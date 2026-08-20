# TS→Go 迁移与兼容收官分析（2026-08-20）

> 一次性解决 TS 版兼容与迁移的全部已知问题（技术 + 用户体验）。本文是设计与决策依据；执行计划见 `../progress/MASTER.md`。

## 1. 方言与路径矩阵（现状）

| 用户起点 | 现状 | 结论 |
|---|---|---|
| TS SQLite 库 | Go 直接接管（AutoMigrate + additive sc2_001~024） | ✅ 已可用（#875/#878） |
| TS PostgreSQL 库 | 理论可直接接管（Go 支持 PG），**但 TS 契约建表 vs Go DDL 的列名差异未实测** | ⚠️ 需验证（Batch G） |
| TS MySQL 库 | Go 无 mysql 驱动；README 声称「metapi-migrate 可迁」是**假的**（migrate_runner 无 MySQL 源）；FAQ 说法矛盾 | ❌ 路径：**TS 自带迁移功能（settings 数据库卡片）把 MySQL 迁到 SQLite/PG → Go 接管**（零 Go 新依赖），文档修复 |
| TS 备份 JSON v2.1 | Go import 不认 TS 格式（只认 `{"tables"}` 与 legacy） | ❌ 兜底路径断裂 → 兼容（Batch F） |
| TS 库比 Go 认知新 | 无检测，`no such column` 崩（hb0730 事故同型翻版风险） | ❌ 反向检测（Batch B） |

## 2. 已核实的 CLI/UI 瑕疵

- `cmd/migrate/main.go:72-74`：`--batch-size` 接受但 `_ = flagBatchSize` 丢弃
- `store/migrate_runner.go:1381`：`buildSummary` 硬编码 `Dialect:"postgres"`（SQLite→SQLite 也显示 postgres）
- `store/migrate_runner.go:384-388`：`--verify` 失败只打 `Verification warning`、**exit 0**——脚本里等于没校验
- `migrate_runner.go:20` 头注释 "13 columns" vs 代码 "14 columns" 陈旧
- 管理 UI `database-section.tsx`：迁移能力被雪藏（头注释 "destructive data migration remains CLI-only"）；`migrateExternalDatabase`（web/src/lib/api/settings.ts）无调用者=死代码；后端已就绪（`POST /api/settings/database/migrate` → 202+taskId，`/api/tasks/{id}` 轮询，`api/events.ts` 已有 `getTask`）

## 3. 启动 UX 缺口

- AutoMigrate 补列有逐条日志（`store: added column`），但**无用户可见汇总**（检测到旧 TS 库 / 共补 N 列）
- TS 侧判龄权威标记：SQLite 库的 `__drizzle_migrations`（hash=sha256(迁移 SQL)、created_at=journal when）；mysql/pg 目标无该表

## 4. 设计决策（用户已确认）

1. **MySQL 路径**：用 TS 自带迁移迁到 SQLite/PG → Go 接管；不给 Go 加 mysql 驱动。文档重写 + 实操指南。
2. **反向检测**：实现。双层——`__drizzle_migrations` 判龄（> Go 认知的最新区间时提示）+ 未知列扫描（TS 有而 Go schema 没有的列 → 警告列出列名与「升级 Go」建议）。警告不阻断启动（TS 新列可能无害，Go 只在 SELECT 到才崩；阻断会锁死旧二进制）。
3. **Admin UI**：把迁移接入现有 database-section（危险确认 + task 轮询进度 + 结果统计），删除/启用 `migrateExternalDatabase`。
4. **TS 备份兼容**：Go import 兼容 TS v2.1（`{version:'2.1', type, accounts:{sites,accounts,accountTokens,tokenRoutes,...}, preferences:{settings:[{key,value}]}}`），作为最后兜底路径。

## 5. 批次与 lane

| 批次 | 内容 | lane | 文件地界 |
|---|---|---|---|
| A CLI 诚实化 | --verify 失败 exit 1；buildSummary 真实方言；--batch-size 实现（multi-row INSERT 批次）；注释修正；测试 | Go 后端 | `store/migrate_runner.go` `cmd/migrate/` |
| B 反向检测 | 判龄 + 未知列扫描 + 启动警告 + 测试（夹具构造 TS 新版库） | Go 后端 | 新 `store/ts_schema_detect.go` + `store/bootstrap.go` |
| C 启动汇总 | AutoMigrate 有实际变更时输出汇总行 | Go 后端 | `store/migrate.go` |
| D 迁移 UI | database-section 接入 migrate + task 轮询 + 危险确认 + i18n | 前端 | `web/src/features/settings/sections/system-info/components/database-section.tsx` + `web/src/lib/api/settings.ts` + i18n |
| E 文档全修 | migration.md 三 personas 重写 + 版本锁定；README/FAQ 矛盾修复；getting-started | 文档 | `docs/migration.md` `README.md` `README_EN.md` `docs/faq.md` `docs/getting-started.md` |
| F TS 备份导入 | import 兼容 v2.1 格式映射 + 测试 | 导入 | `handler/admin/settings_backup.go` `service/backup/` |
| G PG 接管实测 | TS 契约 PG schema 被 Go 接管实测（本地 PG 集群 127.0.0.1:55432） | 验证（主 agent 自跑） | 无代码改动 |

## 6. 验收基线（明卷）

- A：`--verify` 人为制造校验和不匹配 → exit≠0；SQLite→SQLite summary 显示 sqlite；batch-size=100 生成批量 INSERT
- B：夹具构造「TS 新版库」（加 Go 未知列）→ 启动日志出现含列名的警告；正常夹具无警告
- C：老 TS 夹具启动出现「added N columns across M tables」；空库启动无该行
- D：UI 完成 SQLite→PG 真实迁移（本地 PG 55432）+ 进度可见 + 失败态有错误
- E：`go test ./docs/` 门禁全绿；三种 persona 步骤逐一可照做
- F：TS v2.1 样本导入成功且数据正确（含 accounts+preferences）
- G：TS 契约 PG 库启动 Go 不崩、数据可读；若发现缺口记录并归入计划

## 7. 暗卷（验收时亲测，不进任务书）

- 反向检测在**干净新库**上不误报（空库无 __drizzle_migrations、无未知列）
- 迁移 UI 的「同库迁移」被拒绝且提示清晰（sameMigrationTarget 409）
- TS 备份导入对**重复数据**行为明确（覆盖/跳过有文档）
