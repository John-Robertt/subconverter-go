# 决策记录（v1）

本文档用于记录“为什么这么定”的关键决策（rationale），让后续实现/重构/扩展时可以回溯。

约定：
- **规范（SPEC）定义 what**；这里记录 why（背景、取舍、替代方案）。
- 每条决策尽量短：1) 背景 2) 决策 3) 影响（正/负） 4) 相关规范链接。

---

## D001：错误即失败（无宽松模式）

背景：
- 订阅/规则/profile/template 都是远程输入，现实中经常“格式不标准”。
- 但“宽松修复”会产生不可预测的配置，用户很难排错（看似成功，实际行为不对）。

决策：
- v1 只有一种行为：任何错误直接 HTTP 返回结构化错误，要求用户修改远程文件。

影响：
- 优点：可验证、可定位、输出稳定；避免隐性行为。
- 缺点：对用户输入质量要求更高；需要更好的错误信息。

相关规范：
- `../spec/SPEC_HTTP_API.md`
- `../spec/SPEC_SUBSCRIPTION_SS.md`
- `../spec/SPEC_PROFILE_YAML.md`
- `../spec/SPEC_RULES_CLASH_CLASSICAL.md`

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
- `../design/ARCHITECTURE.md`
- `../spec/SPEC_TEMPLATE_ANCHORS.md`
- `../spec/SPEC_RENDER_TARGETS.md`

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
- `../spec/SPEC_RULES_CLASH_CLASSICAL.md`
- `../spec/SPEC_PROFILE_YAML.md`
- `../spec/SPEC_RENDER_TARGETS.md`

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
- `../spec/SPEC_FETCH.md`
- `../spec/SPEC_SECURITY.md`

---

## D005：Proxy 内部身份与展示名分离

背景：
- 真实订阅中同名节点很常见；把 `Name` 同时用作展示名与内部身份，会让策略组成员、渲染器回查、名称冲突处理互相耦合。
- 目标客户端最终仍按节点名/tag 引用节点，因此“展示名唯一化”仍然需要保留，但它不适合继续承担内部主键职责。

决策：
- v1 为每个编译后的节点生成稳定哈希 `proxyID`，作为编译阶段与渲染阶段的内部唯一标识。
- `Name` 保留为展示名与正则匹配对象。
- 显式策略组成员继续只支持“组名 / `DIRECT` / `REJECT` / `@all`”，不支持直接引用单个订阅节点。

影响：
- 优点：内部关联稳定，渲染器职责清晰，节点重命名不再影响内部身份。
- 缺点：Core IR 与 renderer 会有一次结构性迁移；测试数据也需要同步迁移到成员引用模型。

相关规范：
- `../spec/SPEC_DETERMINISM.md`
- `../spec/SPEC_PROFILE_YAML.md`
- `../spec/SPEC_RENDER_TARGETS.md`
- `../design/ARCHITECTURE.md`

---

## D006：链式代理通过派生节点表达，而不是暴露裸 `custom_proxy`

背景：
- 用户需要“自定义代理通过订阅节点访问”的链式能力。
- 若把 `custom_proxy` 直接输出到最终配置，用户会看到一个理论存在但实际不可直连的节点，容易误选，也不利于定位问题。

决策：
- `custom_proxy` 只作为编译输入，不直接进入最终输出。
- 编译器按 `(custom_proxy, subscription_proxy)` 生成链式派生节点，并保留原始订阅节点用于对照诊断。

影响：
- 优点：最终输出中的每个节点都可直接使用；链路关系落在单节点上，渲染器实现简单。
- 缺点：需要新增派生节点命名、排序、自动诊断组等稳定性规则。

相关规范：
- `../spec/SPEC_PROFILE_YAML.md`
- `../spec/SPEC_DETERMINISM.md`
- `../spec/SPEC_RENDER_TARGETS.md`

---

## D007：`CHAIN-` 保留给自动诊断组

背景：
- 链式代理需要一个稳定、可预测的诊断入口，便于用户在客户端里直接比较“原始节点”与“链式派生节点”。
- 如果允许用户自定义同名前缀组名，就需要额外的重命名和歧义处理规则。

决策：
- 自动诊断组统一命名为 `CHAIN-<custom_proxy.name>`。
- `CHAIN-` 作为保留前缀，用户定义的策略组名不得使用。

影响：
- 优点：命名简单、稳定，可直接写进文档与测试。
- 缺点：减少了一小部分用户自定义组名空间。

相关规范：
- `../spec/SPEC_PROFILE_YAML.md`
- `../spec/SPEC_DETERMINISM.md`
