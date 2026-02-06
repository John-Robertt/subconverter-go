# 决策记录（v1）

本文档用于记录“为什么这么定”的关键决策（rationale），让后续实现/重构/扩展时可以回溯。

约定：
- **规范（SPEC）定义 what**；这里记录 why（背景、取舍、替代方案）。
- 每条决策尽量短：1) 背景 2) 决策 3) 影响（正/负） 4) 相关规范链接。

---

## D001：严格模式是唯一模式

背景：
- 订阅/规则/profile/template 都是远程输入，现实中经常“格式不标准”。
- 但“宽松修复”会产生不可预测的配置，用户很难排错（看似成功，实际行为不对）。

决策：
- v1 只提供严格模式：任何错误直接 HTTP 返回结构化错误，要求用户修改远程文件。

影响：
- 优点：可验证、可定位、输出稳定；避免隐性行为。
- 缺点：对用户输入质量要求更高；需要更好的错误信息。

相关规范：
- `SPEC_HTTP_API.md`
- `SPEC_SUBSCRIPTION_SS.md`
- `SPEC_PROFILE_YAML.md`
- `SPEC_RULES_CLASH_CLASSICAL.md`

---

## D002：Core IR 只包含 Proxy/Group/Rule

背景：
- 各客户端的“全局字段宇宙”巨大且不一致（DNS/TUN/脚本等）。
- 试图用 IR 覆盖一切会变成 `map[string]any` 垃圾桶，分支爆炸，无法测试稳定性。

决策：
- v1 IR 只表达转换语义最小交集：节点、策略组、规则；其余全部交给远程模板骨架。

影响：
- 优点：数据结构稳定、渲染器简单、可扩展（加新 target 只写 renderer）。
- 缺点：某些客户端特性需要通过模板预置，不能由 profile 动态描述。

相关规范：
- `ARCHITECTURE.md`
- `SPEC_TEMPLATE_ANCHORS.md`
- `SPEC_RENDER_TARGETS.md`

---

## D003：规则统一为 Clash classical 输入

背景：
- 规则系统是最容易爆炸的地方（各种语法、各种 action 缺省、各种兼容）。
- “每个 target 一套规则解析”会重复实现且行为难以一致。

决策：
- v1 统一规则输入为 Clash classical 子集；ruleset 与 inline rule 共用一个解析器。

影响：
- 优点：复杂度集中、行为一致、错误定位一致。
- 缺点：必须明确子集边界；不支持的规则类型直接报错。

相关规范：
- `SPEC_RULES_CLASH_CLASSICAL.md`
- `SPEC_PROFILE_YAML.md`
- `SPEC_RENDER_TARGETS.md`

---

## D004：允许内网 URL（并显式承担 SSRF 风险）

背景：
- 用户明确需要从内网拉取订阅内容（以及可能的 profile/template/ruleset）。

决策：
- v1 不做“私网 IP 禁止/拦截”；默认通过运行姿势（只监听 127.0.0.1、对外暴露加 token）降低风险。

影响：
- 优点：满足内网场景；实现简单直接。
- 缺点：若误对公网开放，天然具备 SSRF 能力。

相关规范：
- `SPEC_FETCH.md`
- `SPEC_SECURITY.md`

