import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { zhToEnSupplemental } from './i18n.supplement.js';

export type Language = 'zh' | 'en';

const LANGUAGE_STORAGE_KEY = 'app_language';

const zhToEn: Record<string, string> = {
  '管理员': 'Admin',
  '登录令牌无效': 'Invalid admin token',
  '当前 IP 不在管理白名单中': 'Current IP is not in admin allowlist',
  '当前识别到的管理端 IP（由服务端判定）：': 'Current recognized admin IP (server-side detected):',
  '无法连接到服务器': 'Unable to connect to server',
  '请输入管理员令牌后继续。': 'Enter admin token to continue.',
  '管理员令牌': 'Admin Token',
  '管理员入口': 'Admin Access',
  '部署文档': 'Deployment Docs',
  '管理员登录后继续。': 'Continue with admin sign-in.',
  '仅校验本地服务访问权限，不会把令牌发送到第三方。': 'Only checks local service access and never sends the token to a third party.',
  '验证中...': 'Verifying...',
  '登录': 'Sign In',
  '中转站的中转站': 'The Hub of Hubs',
  '把分散的 New API / One API / OneHub 等站点聚合成统一网关，自动发现模型、智能路由、成本更优。': 'Turn fragmented New API / One API / OneHub sites into one unified gateway with auto model discovery, smart routing, and better cost control.',
  '兼容 New API / One API / OneHub / DoneHub / Veloera / AnyRouter / Sub2API': 'Compatible with New API / One API / OneHub / DoneHub / Veloera / AnyRouter / Sub2API',
  '用户名不能为空': 'Username cannot be empty',
  '用户名最多 24 个字符': 'Username can be at most 24 characters',
  '个人信息': 'Profile',
  '右上角头像实时预览': 'Top-right avatar live preview',
  '用户名': 'Username',
  '例如：小王': 'e.g. Alex',
  '头像（Dicebear 随机） · 风格：': 'Avatar (Dicebear Random) · Style:',
  '换一个随机头像': 'Randomize Avatar',
  '取消': 'Cancel',
  '保存': 'Save',
  '关闭': 'Close',
  '打开': 'Open',
  '导航': 'Navigate',
  '重置': 'Reset',
  '全部': 'All',
  '清空': 'Clear',
  '全部已读': 'Mark All Read',
  '控制台': 'Console',
  '仪表盘': 'Dashboard',
  '站点': 'Sites',
  '账号': 'Accounts',
  '未关联站点': 'Unlinked Site',
  '余额': 'Balance',
  '令牌管理': 'Account Tokens',
  '签到记录': 'Check-in Logs',
  '全部签到': 'Check In All',
  '签到中...': 'Checking In...',
  '刷新状态中...': 'Refreshing...',
  '刷新账户状态': 'Refresh Account Status',
  '+ 添加账号': '+ Add Account',
  '路由': 'Routes',
  '模型路由': 'Model Routes',
  '使用日志': 'Usage Logs',
  '暂无使用日志': 'No Usage Logs',
  '可用性监控': 'Availability Monitor',
  '系统': 'System',
  '设置': 'Settings',
  '程序日志': 'System Logs',
  '导入/导出': 'Import/Export',
  '通知设置': 'Notification Settings',
  '暂无通知': 'No Notifications',
  '模型广场': 'Model Marketplace',
  '模型广场刷新进行中': 'Marketplace refresh in progress',
  '已开始刷新模型广场': 'Started refreshing marketplace',
  '没有找到匹配结果': 'No matching results',
  '搜索站点、账号、模型、日志...': 'Search sites, accounts, models, logs...',
  '模型操练场': 'Model Playground',
  '模型测试': 'Model Testing',
  '关于': 'About',
  '关于 Metapi': 'About Metapi',
  '站点文档': 'Site Docs',
  '任务状态已更新': 'Task status updated',
  '会话已过期，请重新登录': 'Session expired, please sign in again',
  '首次使用建议先阅读站点文档：': 'For first-time setup, read site docs: ',
  '首次使用建议先阅读快速上手：': 'For first-time setup, start with Quick Start: ',
  '首次使用建议先阅读快速开始：': 'For first-time setup, start with Quick Start: ',
  '个人信息已保存': 'Profile saved',
  '搜索': 'Search',
  '搜索 (Ctrl+K)': 'Search (Ctrl+K)',
  '通知': 'Notifications',
  '浅色': 'Light',
  '深色': 'Dark',
  '跟随系统': 'Follow System',
  '浅色模式': 'Light Mode',
  '深色模式': 'Dark Mode',
  '退出登录': 'Sign Out',
  '收起侧边栏': 'Collapse Sidebar',
  '展开侧边栏': 'Expand Sidebar',
  '打开导航': 'Open navigation',
  '关闭导航': 'Close navigation',
  '跳到主要内容': 'Skip to main content',
  '系统设置': 'System Settings',
  '站点管理': 'Site Management',
  '账号管理': 'Connection Management',
  '导入 / 导出': 'Import / Export',
  '监控内嵌': 'Embedded Monitor',
  '品牌': 'Brands',
  '全部品牌': 'All Brands',
  '其他': 'Other',
  '供应商': 'Providers',
  '排序方式': 'Sort By',
  '账号数': 'Accounts',
  '令牌数': 'Tokens',
  '延迟': 'Latency',
  '成功率': 'Success Rate',
  '名称': 'Name',
  '收起': 'Collapse',
  '筛选': 'Filter',
  '加载元数据中...': 'Loading metadata...',
  '卡片视图': 'Card View',
  '表格视图': 'Table View',
  '模糊搜索模型名称': 'Fuzzy Search Model Name',
  '覆盖槽位': 'Coverage Slots',
  '去重账号': 'Unique Accounts',
  '平均延迟': 'Avg Latency',
  '共': 'Total',
  '个模型': 'models',
  '个账号': 'accounts',
  '个令牌': 'tokens',
  '个站点': 'sites',
  '令牌': 'Token',
  '复制': 'Copy',
  '复制模型名': 'Copy Model Name',
  '展开': 'Expand',
  '健康': 'Healthy',
  '风险': 'Risk',
  '低延迟': 'Low Latency',
  '基础信息': 'Basic Info',
  '接口能力': 'Endpoint Capabilities',
  '分组计费': 'Group Pricing',
  '暂无标签': 'No Tags',
  '未提供': 'Not Provided',
  '暂无价格元数据': 'No Pricing Metadata',
  '正在加载价格元数据...': 'Loading pricing metadata...',
  '正在加载模型元数据...': 'Loading model metadata...',
  '上游未提供模型说明。': 'Upstream did not provide a model description.',
  '上游未提供文字说明，但已同步标签、能力或价格信息。': 'Upstream did not provide a text description, but tags, capabilities, or pricing data were synchronized.',
  '当前上游仅返回模型 ID，未返回说明字段（常见于很多站点）。': 'The upstream returned only model IDs and no description field (common on many sites).',
  '暂无模型数据': 'No Model Data',
  '请先检查站点与账号状态，然后点击刷新。': 'Check site and account status first, then refresh.',
  '模型名称': 'Model Name',
  '操作': 'Actions',
  '每页条数': 'Rows Per Page',
  '查看': 'Viewing',
  '来自供应商': 'From Provider',
  '品牌的所有模型': 'Brand Models',
  '的模型': 'models',
  '其他未归类的模型': 'Other uncategorized models',
  '所有模型 accountCount 累计值，同一账号在多个模型中会重复计数': 'Cumulative accountCount across all models; same account may be counted repeatedly.',
  '当前筛选范围内去重后的唯一账号数': 'Unique deduplicated accounts in current filters.',
  '刷新选中概率': 'Refresh Selection Probability',
  '自动重建': 'Auto Rebuild',
  '手动增改路由': 'Manual Route Edit',
  '隐藏手动模式': 'Hide Manual Mode',
  '新建群组': 'Create Group',
  '收起群组创建': 'Hide Group Creator',
  '用于创建群组路由（聚合多个上游模型为一个下游模型名，即模型重定向）；自动路由仍会保持开启。': 'Use this to create a group route (aggregate multiple upstream models as one downstream model name); auto-routing remains enabled.',
  '群组显示名（可选，例如 claude-opus-4-6）': 'Group display name (optional, e.g. claude-opus-4-6)',
  '创建群组': 'Create Group',
  '群组已创建': 'Group created',
  '创建群组失败': 'Failed to create group',
  '搜索模型路由...': 'Search model routes...',
  '通道数量': 'Channel Count',
  '排序字段': 'Sort Field',
  '切换排序方向': 'Toggle Sort Direction',
  '升序 ↑': 'Ascending ↑',
  '降序 ↓': 'Descending ↓',
  '手动模式适合高级场景；自动路由仍会保持开启。': 'Manual mode fits advanced scenarios; auto-routing stays enabled.',
  '路由名称（可选，例如 claude 系列）': 'Route name (optional, e.g. Claude Series)',
  '图标（可选，支持 emoji）': 'Icon (optional, supports emoji)',
  '模型匹配（如 gpt-4o、claude-*、re:^claude-.*$）': 'Model pattern (e.g. gpt-4o, claude-*, re:^claude-.*$)',
  '正则请使用 re: 前缀；例如 re:^claude-(opus|sonnet)-4-6$': 'Use re: prefix for regex, e.g. re:^claude-(opus|sonnet)-4-6$',
  '模型映射 key 支持精确匹配、通配符和 re: 正则；按顺序匹配，精确优先。': 'Model mapping keys support exact, glob and re: regex; evaluated in order with exact priority.',
  '规则预览：命中样本': 'Rule preview: matched samples',
  '当前暂无可预览模型，请先同步模型。': 'No preview models yet. Sync models first.',
  '当前规则未命中任何样本模型。': 'Current rule does not match any sample models.',
  '仅展示前 12 个命中样本。': 'Showing only the first 12 matched samples.',
  '映射预览': 'Mapping preview',
  '启用': 'Enabled',
  '禁用': 'Disabled',
  '通道': 'Channel',
  '按模型过滤': 'Filter by model',
  '排序保存中': 'Saving order',
  '删除路由': 'Delete Route',
  '选择账号': 'Select Account',
  '条路由': 'routes',
  '品牌路由': 'Brand Routes',
  '群组': 'Groups',
  '全部群组': 'All Groups',
  '群组路由': 'Group Routes',
  '查看群组路由': 'Viewing group routes',
  '查看未归类品牌路由': 'Viewing uncategorized brand routes',
  '当前精确路由': 'Current exact routes',
  '条，为避免首屏卡顿，默认不自动计算概率，点击“加载选择解释”后按需获取。': 'routes. To avoid first-screen lag, probabilities are not auto-calculated by default. Click "Load Selection Explanation" to fetch when needed.',
  '通配符路由按请求实时决策；概率解释仅在精确模型路由中展示。': 'Wildcard routes are decided in real time; probability explanation is shown only for exact model routes.',
  '通配符路由按请求实时决策；概率解释在当前路由内统一估算。': 'Wildcard routes are decided in real time; probability explanation is estimated uniformly within the current route.',
  '系统会根据模型可用性自动生成路由。优先级按 P0/P1 等桶管理，同一桶内可有多个通道；拖动通道或灰色分隔线即可调整。精确模型路由会自动过滤只支持该模型的账号和令牌。群组路由中的优先级调整会直接回写来源通道。选中概率表示请求到达时该通道被选中的概率。成本来源优先级为：实测成本 → 账号配置成本 → 目录参考价 → 默认回退单价。': 'Routes are auto-generated by model availability. Priorities are managed as P0/P1 buckets, and multiple channels can share the same bucket; drag channels or the gray separators to adjust them. Exact model routes auto-filter accounts and tokens that support that model. Priority edits inside group routes write back to the source channels directly. Selection probability is the chance a channel is chosen. Cost priority: measured cost -> account configured cost -> catalog reference price -> default fallback unit price.',
  '该群组会将多个来源模型聚合为一个对外模型名；这里调整优先级桶时会直接回写来源通道。若某个来源模型被其他群组复用，保存前会提示影响范围。': 'This group aggregates multiple source models into one public model name. Priority bucket edits here write back to the source channels directly. If a source model is reused by other groups, you will be warned about the impact before saving.',
  '通配符路由按请求实时决策；下方优先级桶在整条路由内全局生效，来源模型只作为通道标签展示。': 'Wildcard routes decide per request. The priority buckets below apply globally across the whole route, while source models are shown only as channel labels.',
  '系统会根据模型可用性自动生成路由；精确模型路由会自动过滤只支持该模型的账号和令牌。': 'Routes are auto-generated by model availability; exact model routes auto-filter accounts and tokens that support that model.',
  'P 值是硬优先级，只会在当前最高可用优先级内结合权重、成本和健康度随机选择': 'P value is a hard priority. Selection stays within the highest available priority tier, then chooses randomly using weight, cost, and health signals.',
  '忽略 P 值，按全局顺序依次调用；连续失败 3 次后进入分级冷却': 'Ignore P value and call in global order; after 3 consecutive failures the channel enters staged cooldown.',
  '先避开最近失败或不健康站点，再在稳定池里按顺序轮询；P 值表示轮询顺位': 'Avoid recently failed or unhealthy sites first, then rotate through the stable pool in order; P value means rotation order.',
  '当前策略不看 P 值；如果之后切回其他策略，拖拽保存的顺序仍会保留。': 'This strategy does not use P value; if you switch back to other strategies later, the saved drag order is still kept.',
  '当前策略下，稳定站点会按 P 顺序轮换；不稳定站点会被自动降权或临时避让。': 'Under this strategy, stable sites rotate by P order; unstable sites are automatically downweighted or temporarily avoided.',
  '只要更高优先级还有可用通道，后面的通道本次就不会参与选择。': 'As long as a higher-priority tier still has available channels, later channels will not participate in this selection.',
  '选中概率用于解释当前策略下这一次请求更可能落到哪里；轮询和稳定优先更适合把它当作顺序参考。': 'Selection probability explains where this request is more likely to land under the current strategy; for round robin and stable first it is better treated as an order hint.',
  '代理端点': 'Proxy Endpoints',
  '路由行为': 'Routing Behavior',
  '指标口径': 'Metric Notes',
  'metapi 将多个上游兼容供应商聚合为统一的 OpenAI / Claude 下游兼容入口。': 'Metapi aggregates multiple upstream compatible providers into a unified OpenAI / Claude compatible downstream endpoint.',
  '核心目标：自动签到、自动模型发现、自动路由重建、统一代理可观测性。': 'Core goals: auto check-in, auto model discovery, auto route rebuild, and unified proxy observability.',
  '1. 路由根据模型可用性自动生成。': '1. Routes are auto-generated based on model availability.',
  '2. 当模型或账号发生变更时，路由通道会自动重建。': '2. Route channels are auto-rebuilt when models or accounts change.',
  '3. 手动覆盖配置为可选项，且会尽可能保留。': '3. Manual overrides are optional and kept whenever possible.',
  '4. 成本来源优先级：实测成本 → 账号配置成本 → 目录参考价 → 默认回退单价。': '4. Cost source priority: measured cost -> account configured cost -> catalog reference price -> default fallback unit price.',
  '5. 同站点多通道会进行概率分摊，避免仅因通道数量导致过度偏置。': '5. Multi-channel routes from the same site share probability to avoid bias from channel count alone.',
  '1. 模型广场价格来自上游目录数据，用于展示参考。': '1. Marketplace prices come from upstream catalog data for reference.',
  '2. 路由实测成本来自代理真实请求统计，两者不是同一数据源。': '2. Route measured cost comes from real proxy requests; it is not the same data source.',
  '3. 覆盖槽位是模型维度累计值；去重账号是唯一账号数。': '3. Coverage slots are model-level cumulative counts; unique accounts are deduplicated account counts.',
  '请求超时（': 'Request timed out (',
  '未知账号': 'Unknown Account',
  '未知站点': 'Unknown Site',
  '未知': 'Unknown',
  '未设置': 'Not Set',
  '成功': 'Success',
  '失败': 'Failed',
  '警告': 'Warning',
  '信息': 'Info',
  '异常': 'Error',
  '加载中...': 'Loading...',
  '刷新': 'Refresh',
  '保存中...': 'Saving...',
  '保存失败': 'Save failed',
  '同步中...': 'Syncing...',
  '同步': 'Sync',
  '添加': 'Add',
  '编辑': 'Edit',
  '删除': 'Delete',
  '选择站点': 'Select Site',
  '选择令牌（可选）': 'Select Token (optional)',
  '选择账号后同步站点令牌': 'Select an account to sync site tokens',
  '站点名称': 'Site Name',
  '站点 URL (例如 https://api.example.com)': 'Site URL (e.g. https://api.example.com)',
  '自动检测': 'Auto Detect',
  '检测中': 'Detecting',
  '保存站点': 'Save Site',
  '保存修改': 'Save Changes',
  '编辑站点': 'Edit Site',
  '添加站点': 'Add Site',
  '暂无站点': 'No Sites',
  '点击“+ 添加站点”开始使用。': 'Click "+ Add Site" to start.',
  '重建中...': 'Rebuilding...',
  '发送中...': 'Sending...',
  '导入中...': 'Importing...',
  '创建中...': 'Creating...',
  '更新中...': 'Updating...',
  '登录并添加...': 'Logging in and adding...',
  '添加中...': 'Adding...',
  '同步站点令牌': 'Sync Site Tokens',
  '同步全部账号': 'Sync All Accounts',
  '+ 新增令牌': '+ New Token',
  '保存通知设置': 'Save Notification Settings',
  '发送测试通知': 'Send Test Notification',
  '通知设置已保存': 'Notification settings saved',
  '测试通知已发送': 'Test notification sent',
  '操作失败': 'Operation failed',
  '操作已中止': 'Operation aborted',
  '清空日志': 'Clear Logs',
  '加载更多': 'Load More',
  '全部类型': 'All Types',
  '仅看未读': 'Unread Only',
  '时间': 'Time',
  '类型': 'Type',
  '级别': 'Level',
  '标题': 'Title',
  '内容': 'Content',
  '状态': 'Status',
  '已读': 'Read',
  '未读': 'Unread',
  '标记已读': 'Mark Read',
  '标记中...': 'Marking...',
  '清空中...': 'Clearing...',

  // About page
  '中转站的中转站 — 将你在各处注册的 New API / One API / OneHub 等 AI 中转站聚合为一个统一网关。一个 API Key、一个入口，自动发现模型、智能路由、成本最优。': 'The hub of hubs — aggregate all your New API / One API / OneHub relay sites into one unified gateway. One API Key, one endpoint, with auto model discovery, smart routing, and cost optimization.',
  '核心特色': 'Key Features',
  '统一代理网关': 'Unified Proxy Gateway',
  '一个 Key、一个入口，兼容 OpenAI / Claude 下游格式': 'One Key, one endpoint, compatible with OpenAI / Claude downstream formats',
  '智能路由引擎': 'Smart Routing Engine',
  '按成本、延迟、成功率自动选择最优通道，故障自动转移': 'Auto-selects the optimal channel by cost, latency, and success rate with automatic failover',
  '多站点聚合': 'Multi-Site Aggregation',
  '集中管理 New API / One API / OneHub / DoneHub / Veloera 等': 'Centrally manage New API / One API / OneHub / DoneHub / Veloera and more',
  '自动模型发现': 'Auto Model Discovery',
  '上游新增模型自动出现在模型列表，零配置路由生成': 'New upstream models appear automatically, with zero-config route generation',
  '跨站模型覆盖、定价对比、延迟与成功率实测数据': 'Cross-site model coverage, pricing comparison, latency and success rate metrics',
  '自动签到': 'Auto Check-in',
  '定时签到 + 余额刷新，不再手动操心': 'Scheduled check-in and balance refresh, never miss one again',
  '多渠道告警': 'Multi-Channel Alerts',
  'Webhook / Bark / Server酱 / 邮件，余额不足及时提醒': 'Webhook / Bark / ServerChan / Email — get notified when balance is low',
  '轻量部署': 'Lightweight Deployment',
  '单 Docker 容器，内置 SQLite，无外部依赖': 'Single Docker container with built-in SQLite, no external dependencies',
  '技术栈': 'Tech Stack',
  '高性能 Node.js 后端框架': 'High-performance Node.js backend framework',
  '用户界面库': 'User interface library',
  '端到端类型安全': 'End-to-end type safety',
  '原子化样式框架': 'Utility-first CSS framework',
  '轻量 TypeScript ORM': 'Lightweight TypeScript ORM',
  '零配置嵌入式数据库': 'Zero-config embedded database',
  '项目链接': 'Project Links',
  '数据与隐私': 'Data & Privacy',
  'Metapi 完全自托管，所有数据（账号、令牌、路由、日志）均存储在本地 SQLite 数据库中，不会向任何第三方发送数据。代理请求仅在你的服务器与上游站点之间直连传输。': 'Metapi is fully self-hosted. All data (accounts, tokens, routes, logs) is stored in a local SQLite database and never sent to any third party. Proxy requests travel directly between your server and upstream sites.',
  /* VIS-1 theme accent presets (theme menu) */
  '主题色': 'Accent color',
  '蓝色（默认）': 'Blue (default)',
  '靛蓝（原版）': 'Indigo (original)',
  '青绿（冷静）': 'Teal (calm)',
  /* DENSE-1 table density (theme menu) */
  '表格密度': 'Table density',
  '舒适': 'Comfortable',
  '紧凑': 'Compact',
  '舒适密度': 'Comfortable density',
  '紧凑密度': 'Compact density',
  /* NAV-1 first-run sidebar folding */
  '更多功能': 'More features',
  '更多': 'More',
  /* JSX hardcoded CJK — runtime MutationObserver translates these in EN mode (2026-08-01 sweep) */
  '需要关注': 'Needs attention',
  '暂无趋势数据': 'No trend data yet',
  '总消耗': 'Total spend',
  '总调用': 'Total calls',
  '调用': 'Calls',
  '消耗': 'Spend',
  '页面渲染出错': 'Page failed to render',
  '更新与部署': 'Update & Deploy',
  '反选': 'Invert selection',
  '手动': 'Manual',
  '自定义排序': 'Custom sort',
  '单位成本（可选）': 'Unit cost (optional)',
  '点击按标签过滤': 'Click to filter by tag',
  '推荐': 'Recommended',
  '不支持': 'Not supported',
  '↑ 上移': '↑ Move up',
  '↓ 下移': '↓ Move down',
  '重新绑定': 'Rebind',
  '标签': 'Tag',
  '已用': 'Used',
  '开始': 'Start',
  '结束': 'End',
  '分类': 'Category',
  '建议': 'Suggestion',
  '奖励': 'Reward',
  '首次使用引导': 'First-run guide',
  '尚未产生流量': 'No traffic yet',
  '开始使用': 'Get started',
  '账户数据': 'Account data',
  '累计消耗': 'Cumulative spend',
  '使用统计': 'Usage stats',
  '资源消耗': 'Resource usage',
  '活跃账户': 'Active accounts',
  '性能指标': 'Performance metrics',
  '低': 'Low',
  '高': 'High',
  '一键测速': 'One-click speed test',
  '跳转': 'Jump',
  '管': 'Manage',
  '使用趋势': 'Usage trend',
  '最近使用': 'Recently used',
  '累计请求': 'Total requests',
  '累计成本': 'Total cost',
  '主分组': 'Main group',
  '当前范围汇总': 'Current range summary',
  '请求数': 'Requests',
  '成本': 'Cost',
  '固定窗口对比': 'Fixed-window comparison',
  '留空表示不限': 'Empty = unlimited',
  '留空 = 不拦截。黑名单优先于白名单。': 'Empty = no block. Blacklist wins over whitelist.',
  '填写业务场景、负责人或限制说明': 'Describe use case, owner, or limits',
  '下游密钥': 'Downstream keys',
  '随机': 'Random',
  '请求额度': 'Request quota',
  '成本额度': 'Cost quota',
  '密钥权重': 'Key weight',
  '备注说明': 'Notes',
  '全选': 'Select all',
  '无标签': 'Untagged',
  '暂无下游密钥': 'No downstream keys yet',
  '批量归类 / 标签': 'Bulk categorize / tag',
  '批量追加标签': 'Bulk add tags',
  '+ 新增下游密钥': '+ New downstream key',
  '范围概览': 'Scope overview',
  '当前范围': 'Current scope',
  '授权范围': 'Authorized scope',
  '额度': 'Quota',
  '用量': 'Usage',
  '吗？': '?',
  '个密钥吗？': ' keys?',
  '敏感数据请离线保管': 'Keep sensitive data offline',
  '导出数据': 'Export data',
  '从备份文件恢复数据': 'Restore data from backup',
  '点击重新选择文件': 'Click to re-select file',
  '点击选择文件': 'Click to select file',
  '数据预览': 'Data preview',
  '导出分区': 'Export partitions',
  '注意事项': 'Notes',
  '停止': 'Stop',
  '发送': 'Send',
  '调试': 'Debug',
  '预览': 'Preview',
  '请求': 'Request',
  '响应': 'Response',
  '暂无事件。': 'No events yet.',
  '请选择测试模式': 'Choose a test mode',
  '请选择协议': 'Choose a protocol',
  '开始对话测试': 'Start chat test',
  '清除': 'Clear',
  '对话轮数': 'Chat turns',
  '模式': 'Mode',
  '测试模式': 'Test mode',
  '协议 / 输出格式': 'Protocol / output format',
  '采样参数': 'Sampling params',
  '对话': 'Chat',
  '发送请求': 'Send request',
  '告警去噪与冷静期': 'Alert dedup & cooldown',
  '冷静期（秒）': 'Cooldown (seconds)',
  '微信推送消息支持': 'WeChat push support',
  '通过电子邮件推送提醒': 'Email notifications',
  '端口': 'Port',
  '接收地址': 'Receive URL',
  '扩展渠道': 'Extra channels',
  '飞书': 'Feishu',
  '钉钉': 'DingTalk',
  '企业微信': 'WeCom',
  '重新授权': 'Re-authorize',
  '当前连接': 'Current connection',
  '授权指引': 'Authorization guide',
  '固定回调地址': 'Fixed callback URL',
  '识别结果': 'Recognition result',
  '策略': 'Policy',
  '暂无日志': 'No logs yet',
  '留空表示不过滤': 'Empty = no filter',
  '调试追踪上一页': 'Debug trace previous page',
  '调试追踪下一页': 'Debug trace next page',
  '推测': 'Estimate',
  '目标地址': 'Target URL',
  '执行器': 'Executor',
  '恢复逻辑': 'Recovery logic',
  '暂无追踪详情。': 'No trace details yet.',
  '下游路径': 'Downstream path',
  '最终上游路径': 'Final upstream path',
  '后续新请求会写入调试追踪，不会回补旧请求。': 'New requests are traced; old ones are not backfilled.',
  '采集原始请求/响应头': 'Capture raw request/response headers',
  '保留下游原始头和上游响应头，方便直接对照。': 'Keeps raw downstream request and upstream response headers for direct comparison.',
  '采集请求体和响应体': 'Capture request and response bodies',
  '定向过滤': 'Targeted filter',
  '保留策略': 'Retention policy',
  '保留时长（小时）': 'Retention (hours)',
  '抓取体积上限（字节）': 'Capture size limit (bytes)',
  '最近调试追踪': 'Recent debug traces',
  '上游路径': 'Upstream path',
  '用时': 'Latency',
  '输入': 'Input',
  '输出': 'Output',
  '花费': 'Cost',
  '日志详情': 'Log details',
  '计费过程': 'Billing steps',
  '仅供参考，以实际扣费为准': 'Estimate only; actual charges apply',
  '下游请求路径': 'Downstream request path',
  '未记录': 'Not recorded',
  '上游请求路径': 'Upstream request path',
  '最大费用（可选）': 'Max cost (optional)',
  '最大请求数（可选）': 'Max requests (optional)',
  '备注（可选）': 'Notes (optional)',
  '请输入上方确认语句': 'Type the confirmation text above',
  '当前运行': 'Current run',
  '· 不声明可在此应用内升级': '· No in-app upgrade claim',
  '一行一个关键词，或逗号分隔': 'One keyword per line, or comma-separated',
  '选择动作': 'Choose action',
  '选择单位': 'Choose unit',
  '移除': 'Remove',
  '定时任务': 'Scheduled tasks',
  '保留天数': 'Retention days',
  '常用预设': 'Common presets',
  '新增规则': 'New rule',
  '还没有可视化规则': 'No visualization rules yet',
  '动作': 'Action',
  '协议': 'Protocol',
  '字段路径': 'Field path',
  '批量测活': 'Batch probe',
  '随机生成': 'Random',
  '均衡': 'Balanced',
  '稳定优先': 'Stability first',
  '成本优先': 'Cost first',
  '允许覆盖目标数据库现有数据': 'Allow overwriting existing target data',
  '开始迁移': 'Start migration',
  '维护工具': 'Maintenance tools',
  '会话与安全': 'Session & Security',
  '暂无公告': 'No announcements',
  '权重': 'Weight',
  '最大并发': 'Max concurrency',
  '平台': 'Platform',
  '自定义头': 'Custom headers',
  '以后不再提示': 'Don\'t ask again',
  '确认移除': 'Confirm removal',
  '确认屏蔽': 'Confirm block',
  '分组': 'Group',
  '不限额度': 'Unlimited quota',
  '待补全': 'Pending',
  /* JSX interpolation text-node fragments (2026-08-01 sweep) */
  '个连接吗？': 'connections?',
  '获取。': 'to fetch.',
  '访问': 'access',
  '之类的参数。': 'parameters like these.',
  /* SnapshotExportButton canvas copy — canvas text is not DOM, MutationObserver cannot reach it (2026-08-01) */
  'MetAPI 网关快照': 'MetAPI Gateway Snapshot',
  '生成时间：': 'Generated at: ',
  '总余额': 'Total balance',
  '今日消耗': "Today's spend",
  '24h 请求': '24h requests',
  '24h 成功率': '24h success rate',
  '24h Token': '24h tokens',
  '活跃账号': 'Active accounts',
  '站点消耗 Top': 'Top sites by spend',
  '暂无站点消耗数据': 'No site spend data yet',
  'MetAPI 聚合网关 · TokenDanceLab/metapi-go': 'MetAPI Aggregate Gateway · TokenDanceLab/metapi-go',
/* VChart canvas charts — chart copy renders to canvas, not DOM (2026-08-01) */
  '收入': 'Income',
  '消费': 'Spend',
  '净': 'Net',
  '占比': 'Share',
  '账户数': 'Accounts',
  '加载失败': 'Failed to load',
  '请稍后重试': 'Please retry later',
  '暂无余额历史': 'No balance history yet',
  '暂无延迟数据': 'No latency data yet',
  '暂无模型成本数据': 'No model cost data yet',
  '暂无站点数据': 'No site data yet',
  '刷新账号余额后将自动记录每日快照并展示分析': 'Refresh account balances to record daily snapshots and show analysis',
  '刷新账号余额后将自动记录每日快照并展示趋势': 'Refresh account balances to record daily snapshots and show trends',
  '有代理请求后自动生成成本分布': 'Cost distribution appears after proxy requests',
  '有代理请求后自动生成延迟分布': 'Latency distribution appears after proxy requests',
  '有代理请求后自动生成延迟趋势': 'Latency trend appears after proxy requests',
  '延迟样本量过大，P95 基于有界采样估算': 'Sample too large; P95 is a bounded-sample estimate',
  '数据加载后将自动展示趋势图表': 'Trends appear after data loads',
  '添加站点后将自动展示分布图表': 'Distribution appears after adding sites',
  '余额流入 vs 消费（近 ': 'Income vs spend (last ',
  '天）': ' days)',
  /* chart DOM fragments (2026-08-01) */
  '总成本': 'Total cost',
  '次请求': ' requests',
  '余额分布': 'Balance distribution',
  /* chart spec + object-literal labels sweep (2026-08-01) */
  '无法检查更新': 'Unable to check for updates',
  '已是最新': 'Up to date',
  '图片生成': 'Image generation',
  '视频创建': 'Video creation',
  '正常': 'Normal',
  '轮询': 'Polling',
  '秒': 'Seconds',
  '分钟': 'Minutes',
  '小时': 'Hours',
  '天': 'Days',
  '强制覆盖': 'Force overwrite',
  '文本': 'Text',
  /* SnapshotExportButton button/toast copy (tr() — EN showed Untranslated) */
  '导出快照': 'Export Snapshot',
  '导出中...': 'Exporting...',
  '快照已导出': 'Snapshot exported',
  '导出快照失败': 'Failed to export snapshot',
  '当前浏览器不支持导出 PNG': 'PNG export is not supported in this browser',
  '登录中...': 'Signing in...',
  '模型同步中': 'Syncing models',
  '立即导出到 WebDAV': 'Export to WebDAV now',
  /* JSX interpolation fragments — React splits `title（近 {days} 天）` into
     text fragments; the fragment keys must exist for the observer (2026-08-01) */
  '模型成本分布（近 ': 'Model cost distribution (last ',
  '延迟直方图（近 ': 'Latency histogram (last ',
  'P95 采样截断（': 'P95 truncated (',
  '余额趋势（近 ': 'Balance trend (last ',
  '该 Key 在所选时间范围内没有可用的 tokens 记录': 'No tokens recorded for this key in the selected range',
  '站点分布': 'Site distribution',
  '自定义模式下输入可选。回车发送时将优先使用右侧自定义请求体。': 'Optional in custom mode. Press Enter to send using the custom request body on the right.',
  '输入提示词，或只上传文件后直接发送…（回车发送，Shift+回车换行）': 'Type a prompt, or upload files and send… (Enter to send, Shift+Enter for newline)',
  /* review-wave 2026-08-01: tr()/raw/插值碎片补键（EN 此前显示 Untranslated） */
  '详情': 'Details',
  '不确定': 'Inconclusive',
  '共探测': 'Probed',
  '条': 'items',
  '实时流量': 'Live traffic',
  '在线': 'Online',
  '重连中…': 'Reconnecting…',
  '合计': 'Total',
  '基于当前可见密钥': 'Based on visible keys',
  '账单明细': 'Billing details',
  '回退估算': 'Fallback estimate',
  '覆盖档位': 'Coverage tier',
  '跨站有效价格对比': 'Cross-site price comparison',
  '对比中...': 'Comparing...',
  '对比价格': 'Compare prices',
  '来源': 'Source',
  '缺少真实价格': 'No real price',
  '有效': 'Valid',
  '加载中': 'Loading',
  '严重': 'Severe',
  '公告已发布': 'Announcement published',
  '公告已更新': 'Announcement updated',
  '产品公告': 'Product announcements',
  '+ 新建公告': '+ New announcement',
  '加载中…': 'Loading…',
  '停用': 'Disable',
  '新建公告': 'New announcement',
  '详情链接（可选）': 'Detail link (optional)',
  '查询': 'Search',
  '方法': 'Method',
  '路径': 'Path',
  '映射已生成': 'Mappings generated',
  '条新建': 'created',
  '已修复': 'Fixed',
  '生成映射': 'Generate mappings',
  '标准名': 'Canonical name',
  '上游实际名': 'Actual upstream name',
  '修复中...': 'Fixing...',
  '确认修复': 'Confirm fix',
  '点击切换过滤': 'Click to toggle filter',
  '已选': 'Selected',
  '选择绑定方式': 'Choose binding method',
  '改用高级规则': 'Switch to advanced rules',
  '上下文长度': 'Context length',
  '能力': 'Capabilities',
  '已选中': 'Selected',
  '可选择': 'Selectable',
  '放到新档位': 'Move to new tier',
  '未生成': 'Not generated',
  '退出批量': 'Exit batch',
  '已选择': 'Selected',
  '处理中...': 'Processing...',
  '选择': 'Select',
  '暂无调度任务': 'No scheduled tasks',
  '绑定中...': 'Binding...',
  '触发中...': 'Triggering...',
  '命中即拒绝，优先级高于白名单。用于封禁滥用来源。': 'Rejected immediately, above the allowlist. Use to ban abusive sources.',
  '非空时仅允许列表中的凭证；空列表表示不限制（仍可叠加排除凭证）。': 'When set, only listed credentials are allowed; an empty list means no restriction (exclusions can still apply).',
  '可留空': 'Can be left empty',
  '附件': 'Attachments',
  '加载追踪详情中...': 'Loading trace details...',
  '加载详情中...': 'Loading details...',
  '测试中...': 'Testing...',
  '清理中...': 'Cleaning...',
  '探测中...': 'Probing...',
  '选中概率': 'Selection probability',
  '永久有效': 'Never expires',
  '已识别': 'Detected ',
  '个': 'items',
  '包含:': 'Contains: ',
  '连接:': 'Connection: ',
  '用户:': 'User: ',
  '最近': 'Recent ',
  '次': 'times',
  '已排除': 'Excluded',
  '个凭证': 'credentials',
  '已允许': 'Allowed ',
  '本次会对已选中的': 'This will apply to the selected ',
  '结构有效，版本：': 'Structure valid, version: ',
  '包含分区：': 'Contains partitions: ',
  '/ 书签': '/ bookmarks',
  '最近尝试：': 'Last attempt: ',
  '· 共 $': '· total $',
  '个成员': 'members',
  '已连接': 'Connected ',
  '个。': 'items.',
  '邮箱：': 'Email: ',
  '到期：': 'Expires: ',
  '过滤范围：': 'Filter range: ',
  '显示第': 'Showing ',
  '条，共': ' of ',
  '外置部署 ·': 'External deployment · ',
  '规则': 'Rules ',
  '当前：': 'Current: ',
  '已屏蔽': 'Blocked ',
  '目标：': 'Target: ',
  '版本：': 'Version: ',
  '首次发现：': 'First seen: ',
  '已应用官方预设 ·': 'Official preset applied · ',
  '当前生效：': 'Active: ',
  '个成员 ·': 'members · ',
  '成员摘要（': 'Member summary (',
  '成员': 'members',
  '消耗趋势': 'Spend trend',
  '调用趋势': 'Calls trend',
};

for (const [source, target] of Object.entries(zhToEnSupplemental)) {
  if (!zhToEn[source]) {
    zhToEn[source] = target;
  }
}

const HAS_HAN_RE = /[\u3400-\u9fff]/;
const HAN_BLOCK_RE = /[\u3400-\u9fff]+/g;
const LATIN_OR_DIGIT_RE = /[A-Za-z0-9]/;
const TRANSLATABLE_ATTRS = ['placeholder', 'title', 'aria-label'] as const;
const SKIP_PARENT_SELECTOR = 'script, style, code, pre, kbd, samp, [data-i18n-skip]';
/**
 * Phrase-level replacement table. Single-char keys are matched with a Han
 * boundary check (replaced only when neither neighbour is a Han char), so a
 * bare '\u5171' in '\u5171 12 \u4e2a\u6a21\u578b' translates while '\u4e2d' inside '\u5bfc\u51fa\u4e2d' never
 * shreds the word. Exact whole-string lookup in translateText always applies.
 */
const zhToEnPhrases = Object.entries(zhToEn).sort((a, b) => b[0].length - a[0].length);
function replaceSingleCharPhrase(text: string, source: string, target: string): string {
  // Guard: replaced only when both neighbours are non-Han, so a bare '\u5171' in
  // '\u5171 12 \u4e2a\u6a21\u578b' translates while '\u4e2d' inside '\u5bfc\u51fa\u4e2d' never shreds a word.
  const re = new RegExp(`(?<![\u3400-\u9fff])${source}(?![\u3400-\u9fff])`);
  return text.replace(re, () => target);
}
const textNodeOriginalMap = new WeakMap<Text, string>();
const elementAttrOriginalMap = new WeakMap<Element, Map<string, string>>();

const CJK_PUNCT_TO_ASCII: Record<string, string> = {
  '，': ', ',
  '。': '. ',
  '：': ': ',
  '；': '; ',
  '！': '! ',
  '？': '? ',
  '（': '(',
  '）': ')',
  '【': '[',
  '】': ']',
  '“': '"',
  '”': '"',
  '‘': '\'',
  '’': '\'',
  '、': ', ',
};

function enforceStrictEnglish(text: string): string {
  const normalizedPunctuation = text.replace(/[，。：；！？（）【】“”‘’、]/g, (ch) => CJK_PUNCT_TO_ASCII[ch] ?? ch);
  const strippedHan = normalizedPunctuation.replace(HAN_BLOCK_RE, ' ');
  const compacted = strippedHan.replace(/\s+/g, ' ').trim();
  if (!compacted) return 'Untranslated';
  if (!LATIN_OR_DIGIT_RE.test(compacted)) return 'Untranslated';
  return compacted;
}

function resolveStoredLanguage(): Language {
  const stored = localStorage.getItem(LANGUAGE_STORAGE_KEY);
  if (stored === 'zh' || stored === 'en') return stored;
  return navigator.language.toLowerCase().startsWith('zh') ? 'zh' : 'en';
}

let runtimeLanguage: Language = 'zh';

export function translateText(text: string, language: Language): string {
  if (language === 'zh') return text;
  if (!text) return text;
  if (!HAS_HAN_RE.test(text)) return zhToEn[text] ?? text;
  // Runtime text nodes keep JSX whitespace (`显示第 {n} 条` → fragment
  // '条，共 '); dictionary keys are stored trimmed, so fall back to the
  // trimmed form for exact lookup.
  const exact = zhToEn[text] ?? (text.trim() ? zhToEn[text.trim()] : undefined);
  if (exact) return exact;

  let translated = text;
  for (const [source, target] of zhToEnPhrases) {
    if (!source || source === target) continue;
    if (!translated.includes(source)) continue;
    if (source.length === 1) {
      translated = replaceSingleCharPhrase(translated, source, target);
      continue;
    }
    translated = translated.split(source).join(target);
  }
  if (HAS_HAN_RE.test(translated)) return enforceStrictEnglish(translated);
  return translated;
}

export function tr(text: string): string {
  return translateText(text, runtimeLanguage);
}

type I18nContextValue = {
  language: Language;
  setLanguage: (next: Language) => void;
  toggleLanguage: () => void;
  t: (text: string) => string;
};

const I18nContext = createContext<I18nContextValue | null>(null);

export function I18nProvider({ children }: { children: React.ReactNode }) {
  const [language, setLanguageState] = useState<Language>(() => {
    const resolved = resolveStoredLanguage();
    runtimeLanguage = resolved;
    return resolved;
  });

  useEffect(() => {
    runtimeLanguage = language;
    document.documentElement.setAttribute('lang', language === 'zh' ? 'zh-CN' : 'en');
  }, [language]);

  useEffect(() => {
    const root = document.body;
    if (!root) return;

    const shouldTranslateTextNode = (node: Text): boolean => {
      const parent = node.parentElement;
      if (!parent) return false;
      if (parent.closest(SKIP_PARENT_SELECTOR)) return false;
      if (parent.isContentEditable) return false;
      const value = node.nodeValue || '';
      if (!value.trim()) return false;
      if (!HAS_HAN_RE.test(value) && language !== 'zh') return false;
      return true;
    };

    const processTextNode = (node: Text) => {
      if (language === 'zh') {
        // Restore the original text when switching back. This must not update
        // the original map: otherwise the EN-translated value gets recorded as
        // the original and the Chinese copy is permanently lost (map poisoning).
        const stored = textNodeOriginalMap.get(node);
        if (stored && node.nodeValue !== stored) node.nodeValue = stored;
        return;
      }
      if (!shouldTranslateTextNode(node)) return;
      const current = node.nodeValue || '';
      const stored = textNodeOriginalMap.get(node);
      if (!stored) {
        textNodeOriginalMap.set(node, current);
      } else {
        const expected = translateText(stored, language);
        if (current !== expected && current !== stored) {
          textNodeOriginalMap.set(node, current);
        }
      }
      const source = textNodeOriginalMap.get(node) || current;
      const next = translateText(source, language);
      if (next !== current) {
        node.nodeValue = next;
      }
    };

    const processElementAttrs = (el: Element) => {
      if (language === 'zh') {
        const attrMap = elementAttrOriginalMap.get(el);
        if (attrMap) {
          for (const [attr, original] of attrMap) {
            if (el.getAttribute(attr) !== original) el.setAttribute(attr, original);
          }
        }
        return;
      }
      if (el.matches(SKIP_PARENT_SELECTOR)) return;
      let attrMap = elementAttrOriginalMap.get(el);
      if (!attrMap) {
        attrMap = new Map<string, string>();
        elementAttrOriginalMap.set(el, attrMap);
      }

      for (const attr of TRANSLATABLE_ATTRS) {
        const current = el.getAttribute(attr);
        if (!current || !current.trim()) continue;
        const stored = attrMap.get(attr);
        if (!stored) {
          attrMap.set(attr, current);
        } else {
          const expected = translateText(stored, language);
          if (current !== expected && current !== stored) {
            attrMap.set(attr, current);
          }
        }

        const source = attrMap.get(attr) || current;
        const next = translateText(source, language);
        if (next !== current) {
          el.setAttribute(attr, next);
        }
      }
    };

    const walk = (node: Node) => {
      if (node.nodeType === Node.TEXT_NODE) {
        processTextNode(node as Text);
        return;
      }
      if (node.nodeType !== Node.ELEMENT_NODE) return;

      const el = node as Element;
      processElementAttrs(el);
      for (const child of Array.from(el.childNodes)) {
        walk(child);
      }
    };

    walk(root);
    if (language !== 'en') {
      return;
    }

    const observer = new MutationObserver((records) => {
      for (const record of records) {
        if (record.type === 'characterData') {
          processTextNode(record.target as Text);
          continue;
        }

        if (record.type === 'attributes') {
          processElementAttrs(record.target as Element);
          continue;
        }

        if (record.type === 'childList') {
          for (const node of Array.from(record.addedNodes)) {
            walk(node);
          }
        }
      }
    });

    observer.observe(root, {
      subtree: true,
      childList: true,
      characterData: true,
      attributes: true,
      attributeFilter: [...TRANSLATABLE_ATTRS],
    });

    return () => {
      observer.disconnect();
    };
  }, [language]);

  const setLanguage = useCallback((next: Language) => {
    runtimeLanguage = next;
    setLanguageState(next);
    localStorage.setItem(LANGUAGE_STORAGE_KEY, next);
    document.documentElement.setAttribute('lang', next === 'zh' ? 'zh-CN' : 'en');
  }, []);

  const toggleLanguage = useCallback(() => {
    setLanguage(language === 'zh' ? 'en' : 'zh');
  }, [language, setLanguage]);

  const t = useCallback((text: string) => translateText(text, language), [language]);

  const value = useMemo<I18nContextValue>(() => ({
    language,
    setLanguage,
    toggleLanguage,
    t,
  }), [language, setLanguage, toggleLanguage, t]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n() {
  const value = useContext(I18nContext);
  if (!value) {
    throw new Error('useI18n must be used within I18nProvider');
  }
  return value;
}
