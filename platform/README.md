# Upstream Platform Adapters (上游适配器架构)

本目录承载 Metapi 接入的各类 AI 中转站、官方聚合面板与直接模型上游的统一适配器实现。

---

## 一、 核心适配契约 (PlatformAdapter)

每个适配器实现 platform/adapter.go 中定义的 PlatformAdapter 接口，支持四大维度的能力：

- 自动探测 (URL 识别)
- 账号与会话管理 (Login / VerifyToken)
- 资产与日常运营 (Checkin 自动签到 / GetBalance 余额刷新)
- 模型自动发现与令牌管理 (GetModels / GetAPITokens)

---

## 二、 继承与复用体系 (Struct Embedding)

为降低社区开发新站点的重复代码量，所有适配器通过组合复用基类：

- BaseAdapter: 通用 HTTP、超时、代理与失败分类
- StandardAdapter: OpenAI / Claude / Gemini 原生端点兼容
- OneApiAdapter: OneAPI 及其衍生面板规范 (OneHub / DoneHub)
- NewApiAdapter: NewAPI 规范 (支持平台 UserID 与加密登录回退, AnyRouter)
- Sub2ApiAdapter: 订阅转换型站点

---

## 三、 社区贡献快速上手 (How to Add a New Site)

若社区发现了新的聚合站点或需要定制签到逻辑：

1. 在 platform/ 下创建 provider.go 及对应的 provider_test.go。
2. 继承基类 (如 NewApiAdapter 或 BaseAdapter)，重写差异方法 (如定制的 Checkin 路径)。
3. 在 platform/registry.go 中注册该适配器。
4. 运行 go test ./platform/... 验证。
