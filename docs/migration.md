# 迁移指南：TypeScript → Go

**Last updated**: 2026-08-22

本文覆盖把 TypeScript 版 Metapi 部署迁到 Go 版的完整过程。走哪条路取决于旧版使用哪种数据库：SQLite 与 PostgreSQL 库**停止旧服务后由 Go 直接接管**；MySQL 库必须**先在 TypeScript 版内完成转换**。文末是 `metapi-migrate` CLI 参考、镜像版本锁定与故障排查。

## 三种场景总览

| 旧版数据库 | 路径 | 关键步骤 |
| ---------- | ---- | -------- |
| SQLite | 直接接管（场景 A） | 停 TS → 备份 `hub.db` → bind mount 目录 `chown -R 1001:1001` → 用同样环境变量启动 Go（首启自动补列） |
| PostgreSQL | 直接接管（场景 B） | 停 TS → `pg_dump` 备份 → `DB_TYPE=postgres` + `DB_URL` 指向同一库 → 启动 Go（首启自动迁移） |
| MySQL | 两段式（场景 C） | 在 TS 版「设置 → 数据库」把库迁到 SQLite 或 PostgreSQL → 停 TS → 按场景 A / B 接管 |

为什么 MySQL 特殊：Go 版只支持 SQLite 与 PostgreSQL 两种运行数据库（启动时对未知 `DB_TYPE` 直接报错），`metapi-migrate` 工具同样只在 SQLite 与 PostgreSQL 之间搬运数据，**不支持 MySQL 源**。MySQL 用户不要试图用 `metapi-migrate` 读 MySQL 库——正确的转换入口是 TS 版自己的迁移功能（见场景 C）。

## 场景 A：TS SQLite 库 → 直接接管

Go 启动时自动执行 schema 升级。老 TS 库缺的列——包括 TS 历史迁移加过、Go 早期 registry 漏掉的 34 个 TS-heritage 列（additive 步骤 `sc2_017`~`sc2_024` 补齐，见 v0.16.2 CHANGELOG）——会在首次启动时自动补上，**无需任何手动 DDL**。

### A.1 备份

```bash
cp data/hub.db "backups/hub-pre-migration-$(date +%Y%m%d-%H%M%S).db"
```

这份副本同时是回滚依据。TS 与 Go 使用同样的文件名约定（`DATA_DIR` 默认 `./data`、库文件 `hub.db`）。

### A.2 停止 TS 服务

```bash
docker compose down     # 或 systemctl stop metapi，按原部署方式
```

必须停止后再启动 Go：两边会读写同一个 `hub.db` 文件，不能同时运行。

### A.3 数据目录权限（Docker 部署最常见的坑）

Go 镜像以**非 root 用户（uid 1001）**运行；旧 TS 容器以 root 写入的 bind mount 目录归 root 所有，Go 进程写不进去：

- **bind mount**（`./data:/app/data`）：先在宿主机执行一次

  ```bash
  sudo chown -R 1001:1001 ./data
  ```

  否则启动会被写权限探针拦下，直接报可操作的修复提示（而不是裸 SQLite 错误）：
  `store: data directory "/app/data" is not writable by the current process (uid ...) ... 'chown -R 1001:1001 <host-data-dir>'`。
  只有以 root 直接运行二进制（探针的权限检查对 root 不生效）且目录确实不可写时，
  才会落到裸错误 `attempt to write a readonly database`（带旧库）或 `unable to open database file`（新目录）。
- **命名卷**（`-v metapi_data:/app/data`）：属主自动从镜像继承，**无需 chown**。把 TS 的 `hub.db` 放进卷后直接可用。

### A.4 用同样的环境变量启动 Go 版

环境变量名与 TS 版一致（`DATA_DIR`、`DB_TYPE`、`DB_URL`、`AUTH_TOKEN`、`PROXY_TOKEN`、`ACCOUNT_CREDENTIAL_SECRET` 等），指向同一数据目录即可：

```yaml
# docker-compose 片段（bind mount 指向旧 data 目录）
services:
  metapi:
    image: ghcr.io/deliciousbuding/metapi-go:v0.16.20
    volumes:
      - ./data:/app/data
    environment:
      AUTH_TOKEN: ${AUTH_TOKEN:?AUTH_TOKEN is required}
      PROXY_TOKEN: ${PROXY_TOKEN:?PROXY_TOKEN is required}
      ACCOUNT_CREDENTIAL_SECRET: ${ACCOUNT_CREDENTIAL_SECRET:-}
      DATA_DIR: /app/data
      TZ: ${TZ:-Asia/Shanghai}
    restart: unless-stopped
```

首次启动日志应出现：

```
store: running auto-migration
store: applying additive migration ...   # 仅当有未应用的补列步骤
store: converged legacy schema ...       # 仅当确实收敛了旧 schema（打印本次应用的补列步数）
store: auto-migration complete
```

老库不需要任何手动操作；补列的默认值保证旧行、旧客户端行为不变。

### A.5 验证

```bash
curl http://localhost:4000/ready
# {"status":"ok","database":"ok"}
```

打开 `http://localhost:4000`，用 `AUTH_TOKEN` 登录，确认站点、账号、路由、下游密钥与使用日志都在。

### A.6 回滚

Go 的补列是 additive（只加列、不改旧数据），但最稳妥的回滚还是用 A.1 的副本：停 Go，把 `hub.db` 副本放回原路径，重启 TS 即可。TS 不会因为多出的列受影响。

## 场景 B：TS PostgreSQL 库 → 直接接管

Go 原生支持 PostgreSQL，TS 与 Go 的连接串格式一致（`postgres://` / `postgresql://`）。停掉 TS 后让 Go 指向同一个库即可。

### B.1 备份

```bash
pg_dump -Fc metapi -f "backups/hub-pre-migration-$(date +%Y%m%d-%H%M%S).dump"
```

### B.2 停止 TS 服务

同 A.2。TS 与 Go 不能同时连同一个库跑调度任务。

### B.3 让 Go 指向同一库

设置 `DB_TYPE=postgres` 与 `DB_URL`（或 `DATABASE_URL`，二者等效）为同一连接串。只提供 `postgres://` URL 时不写 `DB_TYPE` 也会自动推断为 postgres：

```
DB_TYPE=postgres
DB_URL=postgres://USER:PASS@HOST:5432/metapi?sslmode=require
```

### B.4 启动 Go 版并观察首次迁移

镜像与 compose 结构同场景 A（把 `DB_TYPE` / `DB_URL` 加进 environment）。首次启动对 PG 库执行同样的自动迁移（基础 DDL 幂等 + additive 补列），日志同样以 `store: auto-migration complete` 收尾。

> PG 直接接管的列兼容由启动自动迁移处理。若启动或查询报错（如 `no such column`），先看日志定位：这类报错通常意味着 TS 库比当前 Go 版本新（TS 后续迁移加过列而 Go 还没跟上），把 Go 升级到更新版本（见「版本锁定与升级」）。其它错误从日志的 `store: ...` 行入手，必要时回滚到 B.1 的备份。

### B.5 验证

`curl http://localhost:4000/ready` → `{"status":"ok","database":"ok"}`；管理界面核对站点、账号与路由。

### B.6 回滚

停 Go，用 B.1 的备份恢复库（`pg_restore`），重启 TS。

## 场景 C：TS MySQL 库 → 两段式迁移

**Go 版不支持 MySQL，`metapi-migrate` 也不支持 MySQL 源。** MySQL 用户的路径是：

1. 在 TS 版管理界面里用其内置迁移功能，把库迁成 SQLite（或 PostgreSQL）；
2. 停止 TS；
3. 按场景 A（SQLite）或场景 B（PostgreSQL）接管。

### C.1 TS 版操作要点

TS 版管理界面的「设置 → 数据库」卡片（标题「数据库迁移（SQLite / MySQL / PostgreSQL）」）提供完整的库间迁移功能，源库是 TS 当前正在运行的库（即你的 MySQL 库）：

1. 打开 TS 管理界面 → **设置**，找到 **数据库迁移** 卡片。
2. **目标方言**选 `SQLite`（推荐：迁移产物是一个 SQLite 文件，之后按场景 A 接管）；也可以选 `PostgreSQL`（之后按场景 B 接管）。
3. 填目标连接：
   - 选 `SQLite` 时填目标文件路径（如 `./data/hub-go.db` 或 `file://` 绝对路径）；
   - 选 `PostgreSQL` 时填 `postgres://` 连接串（TS 界面提供简写模式：host / user / password）。
4. 点 **测试连接**，确认目标可达。
5. 保持 **允许覆盖目标数据库现有数据** 勾选（默认勾选），点 **开始迁移**。
6. 迁移完成后卡片会显示各表行数（站点 / 账号 / 令牌 / 路由 / 通道 / 设置）。此时得到：
   - 一个 SQLite 文件（步骤 3 填的路径），或
   - 一个已写入数据的 PostgreSQL 库。

> SQLite 产物文件名若不是 Go 默认的 `hub.db`：把该文件复制到 Go 的 `DATA_DIR` 下并命名为 `hub.db`，或直接用 `DB_URL` 指向该文件路径（Go 的 `DB_URL` 支持普通文件路径与 `sqlite://` 前缀）。
>
> 「保存为运行数据库（重启后生效）」按钮不是本路径必需——它只是让 TS 自己换库运行；迁移完成后直接停 TS 即可。

### C.2 停 TS、按场景 A / B 接管

C.1 完成后停止 TS 服务，然后按场景 A（SQLite 产物）或场景 B（PostgreSQL 产物）继续。

### C.3 额外保险：JSON 备份

TS 版还提供 JSON 备份（Schema v2.1）导出/导入，可作为 MySQL 数据的离线副本；但主路径是 C.1 的库间迁移，不要用 JSON 导出代替迁移。

> 转换必须在 TS 版还在运行、还能连上 MySQL 时完成。停掉 TS 之后，没有任何本仓库工具能直接读 MySQL 库。

## metapi-migrate CLI 参考

`metapi-migrate`（`make migrate-build` 构建）在 SQLite 与 PostgreSQL 之间搬运 18 张应用表的数据（含逐列类型转换与 JSON 列序列化，行为对齐 TS 原版 databaseMigrationService）。支持的方向：

| 方向 | 说明 |
| ---- | ---- |
| SQLite → PostgreSQL | 正向迁移 |
| PostgreSQL → SQLite | 反向迁移 |
| SQLite → SQLite | 复制 / 方言校验 |
| PostgreSQL → PostgreSQL | **不支持**（工具直接报错拒绝） |
| MySQL → 任意 | **不支持**（工具只能读 SQLite 文件或 `postgres://` 源） |

### Flags

| Flag | 说明 |
| ---- | ---- |
| `--from` | 源库：SQLite 路径（`sqlite://path` 或普通路径）或 `postgres://` URL |
| `--to` | 目标库，编码规则同上 |
| `--overwrite` | 迁移前按 FK 安全顺序清空目标数据（默认 true，与 TS 一致） |
| `--dry-run` | 只打印迁移计划，不写数据 |
| `--progress` | 每 100 行打印一次进度 |
| `--verify` | 迁移后做行数 + 校验和核对 |
| `--batch-size N` | 每条多行 INSERT 最多 N 行（默认 1 = 逐行插入，与 TS 默认一致）；兼容 TS CLI 保留 |

### 示例：SQLite → PostgreSQL

```bash
# 先 dry-run 看迁移计划
./metapi-migrate \
  --from sqlite://data/hub.db \
  --to 'postgres://USER:PASS@HOST:5432/metapi?sslmode=require' \
  --dry-run

# 实际迁移（overwrite 默认开启），带进度与校验
./metapi-migrate \
  --from sqlite://data/hub.db \
  --to 'postgres://USER:PASS@HOST:5432/metapi?sslmode=require' \
  --overwrite \
  --progress \
  --verify
```

`--verify` 迁移完成后输出 `Verifying checksums...`，确认无误时打印 **`All checksums match.`**。
校验失败时打印 `Verification failed: ...`（含失败表与 source/target 校验和）并以非零码退出。
注意 `--verify` 是**随本次迁移执行**的核对（拷贝完成后立即比对源快照与目标）；工具没有
独立的「只复核已迁移目标」模式——迁移完成后对目标库的外部改动，要靠备份/再次迁移发现。

### 示例：PostgreSQL → SQLite

```bash
./metapi-migrate \
  --from 'postgres://USER:PASS@HOST:5432/metapi?sslmode=require' \
  --to sqlite://data/hub-go.db \
  --overwrite \
  --progress \
  --verify
```

### 说明

- 迁移只搬运数据行；目标库的 schema 由迁移器调用 `AutoMigrate` 建立（基础 DDL + additive 补列一步到位，迁移日志可见 `store: converged legacy schema`）；迁移完成后启动 Go 服务是幂等的。
- settings 表的运行时键（`db_type`、`db_url`、`db_ssl`）会被过滤，不会覆盖目标库的连接配置。
- 源 SQLite 文件只读、不会被修改；目标库已有数据且未开 `--overwrite` 时迁移会拒绝执行。

## 版本锁定与升级

生产环境**不要用 `:latest`**，把镜像固定到具体版本标签：

```yaml
image: ghcr.io/deliciousbuding/metapi-go:v0.16.20
```

升级步骤：

1. **读 [CHANGELOG](../CHANGELOG.md)**：确认目标版本对老库 / 迁移的改动（例如 v0.16.2 补齐了 34 个 TS-heritage 列，老 TS 库无需手动操作）。
2. **备份**：SQLite 复制 `hub.db`；PostgreSQL `pg_dump -Fc`。
3. **换 tag**：改 compose 的 `image:` 后 `docker compose up -d`（或 `docker pull` 新 tag 后重建容器）。
4. **观察日志**：启动日志应出现 `store: running auto-migration` / `store: auto-migration complete`，然后 `curl http://localhost:4000/ready` 确认 `database` 为 `ok`。

schema 升级是 forward-only（只加列 / 表，不做自动降级）；回退版本时旧二进制会忽略新列，数据回滚靠步骤 2 的备份。

## 故障排查

| 现象 | 处理 |
| ---- | ---- |
| 启动报 `store: data directory ... is not writable by the current process` | 数据目录权限：bind mount 目录归 root 所有（#849/#875 探针，报错自带修复提示）。宿主机执行 `chown -R 1001:1001 ./data`；全新部署改用命名卷免配置 |
| 启动报 `attempt to write a readonly database` 或 `unable to open database file` | 同上（以 root 运行绕过探针时才会看到的裸 SQLite 错误）。宿主机执行 `chown -R 1001:1001 ./data`；全新部署改用命名卷免配置 |
| 查询报 `no such column: <列名>` | 老 TS 库缺列，正常情况首启自动补列（additive `sc2_001`~`sc2_024`）。**仍出现**说明 TS 库比当前 Go 版本新（TS 后续迁移加过列），升级 Go 到更新版本 |
| `metapi-migrate` 报 `target database already contains data` | 目标库已有数据且未开 `--overwrite`。确认覆盖意图后加 `--overwrite`（默认即开启） |
| `metapi-migrate` 报 `unsupported direction: PostgreSQL source to PostgreSQL target` | PG→PG 数据搬运不支持；PG 库用场景 B 直接接管 |
| `metapi-migrate` 对 MySQL URL 报错 | 工具不支持 MySQL 源；MySQL 库走场景 C（TS 版内迁移） |
| 启动报 `AUTH_TOKEN is required` | 必填环境变量缺失，见 [配置参考](configuration.md) |
| 迁移后数据对不上 | 先看 `metapi-migrate --verify` 的输出；仍不对就用 A.1 / B.1 的备份回滚重来 |

## 附：Go 版内的其它迁移

与 TS→Go 迁移无关、但同属「迁移」主题的两个入口：

- **Additive 列升级**：由服务启动自动执行，记录在 `schema_migrations` 表；forward-only，失败不会写记录、下次启动自动重试。运维细节见上文各场景与「版本锁定与升级」。
- **Settings schema v1 升级**：Go 版管理界面的「设置 → 定时任务 → 升级旧版计划」，或调用 `GET /api/settings/migration/preview` + `POST /api/settings/migration/apply`；additive 且事务化，重复执行返回 `applied: 0`（接口见 [API 文档](api.md)）。
