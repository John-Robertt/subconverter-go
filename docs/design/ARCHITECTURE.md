# subconverter-go 架构设计（v1）

本文档只回答两件事：
1) **这个项目的目标/边界是什么**（做什么、不做什么）。  
2) **围绕目标做出的关键架构约束**（保证简单、可验证、可扩展）。  

字段/语法/行为的“可执行规范”放在 `../spec/` 下的 `SPEC_*.md`；实现侧的最小目录拆分建议见 `DESIGN_CODE_LAYOUT.md`（都在 `../README.md` 汇总）。

---

## 1. 项目目标（Goals）

### 1.1 核心目标

构建一个 **Go 单二进制** 的 HTTP 服务，将“SS 订阅”转换为目标客户端可直接导入/使用的内容：

- `mode=config`：输出 **配置文件**（节点 + 策略组 + 规则），由远程模板提供静态骨架，服务端只做锚点注入。
- `mode=list`：输出 **纯节点列表**（ss://…），用于只取节点的场景。

v1 优先支持：
- 输入：Shadowsocks（SS）订阅（base64 或明文 ss:// 列表）。
- 输出目标（target）：`clash`、`shadowrocket`、`surge`。
- 规则解析：以 **Clash classical 规则行** 作为统一规则输入格式（远程 ruleset 与 inline rule 共用一套解析器）。

### 1.2 严格模式（唯一模式）

服务端是“编译器”而不是“修复器”：
- 任何解析失败、语义不完整、能力不支持、模板锚点异常、引用不成立等问题，**一律 HTTP 返回错误**。
- 不提供“宽松模式/warnings”作为默认退路（避免生成“看似成功但实际不可用”的配置）。

### 1.3 可扩展目标

允许在不重写核心的前提下扩展：
- 新输入格式：增加新的订阅解析器。
- 新输出目标：增加新的渲染器（renderer）。
- 新规则来源：增加规则集格式解析器（但 v1 先统一 Clash classical）。

扩展的前提是：核心数据结构稳定、行为确定、错误可定位。

---

## 2. 非目标（Non-goals）

为保持简单与可验证，v1 明确不做：
- 覆盖所有客户端的所有字段/特性（例如各客户端的 DNS/TUN/脚本等“全局配置宇宙”）。
- 在服务端执行任何远程内容（不支持可执行模板语言；不执行 JS/Lua/脚本）。
- “自动修复”用户的远程 profile/template/ruleset（出错就报错，用户自己修）。
- 依赖客户端运行时去拉 ruleset（v1 规则集在服务端拉取并展开成最终规则文本）。

---

## 3. 关键架构约束（Key Constraints）

### 3.1 数据驱动：稳定的“核心中间态”（Core IR）

不同客户端的配置格式不同，这是事实；但它们共享的“转换语义”很小：
- **节点（Proxy）**
- **策略组（Group）**
- **规则（Rule）**

因此本项目的核心中间态（IR）只表达这三类稳定语义；其余差异化的全局字段不进入 IR，由模板承载。

> 重要：不要为了“覆盖一切”把 IR 做成一个装满可选字段的垃圾桶。那会让算法变复杂、测试变困难、输出不可预测。

### 3.2 模板只做骨架：锚点注入（Anchor Injection）

远程模板是**纯文本骨架**，服务端只做固定锚点替换：
- `#@PROXIES@#`
- `#@GROUPS@#`
- `#@RULES@#`

锚点约束（v1 强制）：
- 每个锚点 **必须出现且仅出现一次**。
- 锚点必须 **独占一行**（避免替换后破坏语法）。
- 对 YAML（Clash）模板需要保留/继承锚点行的缩进，以保证注入后的缩进合法。

### 3.3 Profile 用 YAML，但“用法像 Rules.ini”

为了易用性，profile 不做深层嵌套结构，而采用“有序指令列表”的风格：
- `custom_proxy_group`: string 列表（保序）
- `ruleset`: string 列表（保序）
- `rule`: string 列表（保序）
- `template`: target -> URL 映射

这样既保留 Rules.ini 的可读性，又避免 INI 解析对“重复 key/顺序”的天然不友好。

### 3.4 统一规则输入：Clash classical

远程 ruleset 与 inline rule 都按 Clash classical 规则行解析，构建成 IR 的 `Rule` 列表，再由各 target renderer 输出到对应语法。

这条约束的目的是把复杂度锁死在一个地方：**规则解析器**。

---

## 4. 端到端行为（High-level Pipeline）

整个服务固定为四段流水线（每段都必须可单测、可定位错误）：

1) Fetch：拉取订阅/profile/template/ruleset（超时、大小上限、缓存、并发去重）
2) Parse：分别解析输入文本为结构化数据（订阅->Proxy；profile->ProfileSpec；ruleset->Rule）
3) Compile：将 ProfileSpec + Proxies 编译为最终 IR（组生成、规则拼装、语义校验）
4) Render：按 target 输出文本；`mode=config` 再注入模板锚点得到最终配置文件

任何阶段失败都返回结构化错误，并附带尽可能多的定位信息（URL/行号/片段/阶段）。

---

## 5. 安全与运行姿势（Security & Ops Defaults）

### 5.1 远程内容只解析不执行

- profile/template/ruleset 都是“数据输入”，不允许被执行。
- 通过“锚点注入”替代可执行模板，避免把服务端变成远程代码执行器。

### 5.2 允许内网 URL（用户已选择）

本项目需要支持访问内网资源（订阅/profile/template/ruleset 均允许为内网 URL）。

允许内网意味着 SSRF 风险上升。为避免默认踩雷，建议运行默认值：
- 服务默认仅监听 `127.0.0.1`（需要显式配置才允许对外暴露）。
- 若对外暴露，必须提供简单鉴权（例如 token），否则此服务可被滥用为内网探测器。

更细的安全策略与默认运行姿势见《安全与内网访问规范》：`../spec/SPEC_SECURITY.md`。

---

## 6. 可验证性（Determinism & Testability）

为了让用户能够 diff/回滚/排错，输出必须尽量确定：
- 节点重命名、去重、排序规则必须稳定且可配置（但默认要稳定）。
- 同一份输入（含远程资源内容一致）应产生完全相同的输出。

测试策略原则：
- 使用 golden test：给定订阅/profile/template/ruleset，断言输出文本字节级一致。
- 对解析器进行恶意/随机输入测试，确保“不崩溃 + 错误可定位”。

---

## 7. 规范文档地图（v1）

如果你要“实现这个编译器”，建议按下面顺序读（由外到内）：
- 入口与边界：`ARCHITECTURE.md`
- 对外接口：`../spec/SPEC_HTTP_API.md`
- 远程拉取：`../spec/SPEC_FETCH.md`
- 输入解析：`../spec/SPEC_SUBSCRIPTION_SS.md`
- profile（编译指令）：`../spec/SPEC_PROFILE_YAML.md`
- 规则语法（统一输入）：`../spec/SPEC_RULES_CLASH_CLASSICAL.md`
- 编译稳定性（去重/命名/排序/managed-config URL 序列化）：`../spec/SPEC_DETERMINISM.md`
- 渲染到各 target：`../spec/SPEC_RENDER_TARGETS.md`
- 模板注入与锚点约束（含 Surge managed-config 重写/插入）：`../spec/SPEC_TEMPLATE_ANCHORS.md`
- 安全与内网访问：`../spec/SPEC_SECURITY.md`
- 代码目录与模块职责（最小实现拆分建议）：`DESIGN_CODE_LAYOUT.md`
