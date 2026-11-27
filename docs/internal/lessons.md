# 反复出现的缺陷与回归断言

记录能用输入和输出说明的根因。同类问题复现 3 次，即晋升为工具门禁或工程标准。
下面列出每类缺陷应长期保留的对照。

| 根因 | 回归断言 |
| --- | --- |
| 同一判定分散在多个入口，首个报告只经过其中之一 | 将触发条件送入所有受影响的分支，同时保留一个应正常通过的相邻场景。入口选择见[贡献规范](../../CONTRIBUTING.md#重构与审查)。 |
| 把 Responses 的 `incomplete` 当成传输中断，或当成模型完整输出 | 发送带 `max_output_tokens` 和 usage 的 `response.incomplete`，分别检查客户端终态、请求记录和计数。再发送 `response.failed` 或在终态前断流，验证各自的错误。见 [WS 终态用例](../../handler/proxy/direct_responses_ws_lifecycle_test.go)。 |
| 通用 JSON 解码把整数转为 `float64`，经过协议桥接后低位丢失 | 用 `9007199254740993` 穿过真实转发入口，在上游收到的请求中核对整数字面值。见[协议精度用例](../../handler/proxy/direct_protocol_precision_test.go)。 |
| OAuth 池预览把候选成员认作已选账号，成员的冷却状态随之错配 | 建立含两个成员的池，分别断言预览没有已选账号、清除冷却只影响指定成员。见[双数据库成员用例](../../handler/admin/oauth_member_cooldown_test.go)。 |
| 用标量计数的校验规则校验整个 Gemini `usageMetadata`，连合法的模态明细数组也被拒绝 | 为同一组 token 总数分别附上和省略 `promptTokensDetails` 数组，检查转发结果和计数一致；再把标量计数改成数组，单独断言无效输入的行为。 |
| 本机构建漏掉其他平台的条件文件，普通跨平台构建又漏掉 `_test.go` | CI 的 `vet` 对选中包分别使用 Linux、Windows 和 macOS 的构建条件，检查各平台测试文件能否通过类型检查。 |
