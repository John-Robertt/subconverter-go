# 代码目录与模块职责（v1 设计建议）

本文档把 `docs/` 里的规范落到“最小可实现”的 Go 代码拆分上：哪些包负责什么、数据结构放哪里、流水线如何串起来。

目标不是搭框架，而是让实现时**不需要发明新的抽象**。

---

## 1. 先定数据结构：Core IR

本项目的核心中间态（IR）只需要三类数据（见 `ARCHITECTURE.md`）：
- Proxy（节点，v1 仅 `ss`）
- Group（策略组，v1 仅 `select/url-test`）
- Rule（规则，v1 仅 Clash classical 子集）

建议把这些类型放在一个极薄的包里（避免循环依赖），例如：

- `internal/model`
  - `Proxy`：`Name, Type, Server, Port, Cipher, Password, Plugin, PluginOpts, UDP, TFO ...`
  - `Group`：`Name, Type, Members[] | Regex/URL/Interval/Tolerance ...`
  - `Rule`：`Type, Value, Action, NoResolve`
  - `AppError`：与 `SPEC_HTTP_API.md` 对齐的结构化错误（`code/message/stage/url/line/snippet/hint`）

实现层的所有模块都只做两件事：
1) 把文本解析为这些结构体  
2) 把这些结构体渲染回文本  

---

## 2. 端到端流水线（按阶段分包）

规范里已经把流程固定为：Fetch -> Parse -> Compile -> Render（见 `ARCHITECTURE.md`）。

建议按阶段拆包（每段都可单测、可返回结构化错误）：

- `internal/httpapi`
  - 解析 GET `/sub` 与 POST `/api/convert`
  - 参数校验（`mode/target/sub/profile/encode`）
  - 统一错误输出（JSON）与成功输出（text/plain）

- `internal/fetch`
  - 仅做 http/https 拉取（超时/大小上限/重定向/UTF-8 校验），完全按 `SPEC_FETCH.md`
  - （实现建议）对相同 URL 的并发请求 singleflight 去重

- `internal/sub/ss`
  - SS 订阅解析：raw/b64 自动识别、ss:// 两种形态、plugin query 处理
  - 只产出 `[]model.Proxy`，按 `SPEC_SUBSCRIPTION_SS.md`

- `internal/profile`
  - profile YAML 解析与校验（`version/template/public_base_url/custom_proxy_group/ruleset/rule`）
  - 只产出一个“ProfileSpec”结构体（可以放在 `internal/profile` 包内），按 `SPEC_PROFILE_YAML.md`

- `internal/rules`
  - Clash classical 规则解析器（行 -> `model.Rule`）
  - ruleset 文件解析（支持缺省 ACTION），按 `SPEC_RULES_CLASH_CLASSICAL.md`

- `internal/compiler`
  - 把 `ProfileSpec + []Proxy` 编译为最终 IR：`[]Proxy + []Group + []Rule`
  - 负责：去重/命名冲突处理/排序/`@all` 展开/引用校验/兜底 MATCH 校验
  - 行为按 `SPEC_DETERMINISM.md` 与 `SPEC_PROFILE_YAML.md`

- `internal/render`
  - 目标渲染：把 IR 渲染为三段文本块（proxies/groups/rules）
  - 可按 target 分子包：`internal/render/clash`、`internal/render/surge`、`internal/render/shadowrocket`
  - 行为按 `SPEC_RENDER_TARGETS.md`

- `internal/template`
  - 模板锚点校验、section 校验（Surge/Shadowrocket）、缩进继承注入
  - Surge `#!MANAGED-CONFIG` 的插入/重写（URL 生成由编译器或专门函数完成，但规则按 `SPEC_TEMPLATE_ANCHORS.md`）

---

## 3. 你应该避免的“模块化陷阱”

v1 最容易走偏的点是过早“插件化/可配置化”。建议避免：
- 把所有字段抽象成 `map[string]any`（会让错误定位与稳定性崩掉）
- 把 IR 做成“覆盖所有客户端字段”的巨型结构（会引入大量分支与特殊情况）
- 过早引入模板语言或 DSL（直接把服务端变成执行器，安全与可测试性会恶化）

v1 的正确扩展方式：
- 新输入：新增一个 parser（产出同一个 `[]Proxy`）
- 新 target：新增一个 renderer（消费同一个 IR）
- 新规则类型：扩展 `internal/rules` 的 parser + `internal/render/*` 的映射（缺任何一段都必须报错）

---

## 4. 测试建议（最小但够硬）

- 单测：
  - `internal/sub/ss`：覆盖两种 ss:// 形态、b64/raw 自动识别、错误行号
  - `internal/rules`：覆盖字段数量、no-resolve、MATCH 限制、缺省 ACTION
  - `internal/template`：覆盖锚点缺失/重复/不独占、section 错误、缩进继承

- 端到端 golden test：
  - 直接复用 `docs/materials/`（见 `docs/materials/README.md`）
  - 固定输入 -> 断言输出字节级一致（尤其是 `#!MANAGED-CONFIG` URL 序列化与排序规则）

