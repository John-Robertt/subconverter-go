# 开发文档方法（渐进式 / 可回溯 / 可验证）

本文档定义本项目的“开发文档与验证方式”：怎么分阶段实现、每一阶段如何验收、改动如何回溯与约束。

目标：让实现像写编译器一样可控——每次只增加一个小能力，并立刻用测试材料验证；避免最后才发现“一堆能跑但不可靠的边角行为”。

---

## 1. 文档分层（别把“规范/设计/计划”混在一起）

本仓库的 `docs/` 约定三类文档：

1) **规范（SPEC_*.md）**：定义“必须满足的外部可观察行为（what）”  
   - 例如：HTTP 的状态码/错误结构、SS 解析规则、规则语法、模板锚点、输出稳定性。
   - 规范是验收基准：写代码必须对齐规范，测试必须覆盖规范。

2) **设计（DESIGN_*.md / ARCHITECTURE.md）**：解释“为什么这样做、最小拆分如何落地（why/how）”  
   - 例如：为什么 IR 只包含 Proxy/Group/Rule、如何拆成包、流水线阶段如何串联。

3) **开发与验证（本文）**：定义“按什么阶段交付、每阶段怎么验证（when/verify）”  
   - 不定义新行为，只引用 SPEC 作为验收标准。

---

## 2. 分阶段交付（每阶段都可单测/可联调）

以下阶段顺序是“由外到内 + 先把不确定性锁死”的实现策略。每一阶段都有明确的 **Definition of Done**（DoD），并且要求增加对应测试。

### 阶段 0：项目骨架与统一错误类型

范围：
- 建立 Go module、最小 server、统一错误结构（对齐 `SPEC_HTTP_API.md`）。

DoD：
- `go test ./...` 能跑（即便测试数量很少）。
- 代码里有一个统一的 `AppError`（或同等结构）能序列化成规范 JSON。

### 阶段 1：Fetch（先把“远程不可靠”问题收敛）

规范：
- `SPEC_FETCH.md`
- `SPEC_HTTP_API.md`（错误码/阶段字段）

DoD（必须测出来）：
- 仅允许 http/https；其它 scheme 返回 `400 INVALID_ARGUMENT`
- 超时、大小上限、重定向次数上限都生效
- 非 UTF-8 内容返回 `FETCH_INVALID_UTF8`
- 错误必须带 `stage=fetch_*` 与 `url`

建议测试：
- `httptest.Server` 覆盖：超大 body、慢响应、重定向链、返回非 UTF-8 字节。

### 阶段 2：SS 订阅解析（只做“解析”，不做“修复”）

规范：
- `SPEC_SUBSCRIPTION_SS.md`

DoD：
- raw/b64 自动识别可复现
- 支持两种 ss:// 形态（userinfo-base64 / 全量 base64）
- 错误定位必须带 `url/line/snippet`（`stage=parse_sub`）

建议测试：
- 订阅样例 + 人工构造的坏行（非法 base64 / 非 ss:// / 缺字段 / 非法端口）。

### 阶段 3：规则解析（把“规则语法”锁死）

规范：
- `SPEC_RULES_CLASH_CLASSICAL.md`

DoD：
- ruleset 文件允许缺省 ACTION（由 `ruleset: ACTION,URL` 补齐）
- inline rule 必须显式 ACTION
- ruleset 文件中出现 `MATCH` 必须报错

建议测试：
- 覆盖字段数量、`no-resolve`、`IP-CIDR` 合法性、缺省 ACTION 与歧义写法。

### 阶段 4：Profile 解析

规范：
- `SPEC_PROFILE_YAML.md`

DoD：
- `version/template/public_base_url/custom_proxy_group/ruleset/rule` 完整解析与最小校验
- `custom_proxy_group` 子集（select/url-test）解析正确；引用与 regex 错误可定位

建议测试：
- 好 profile + 多种坏 profile（缺字段/枚举值错/URL 非法/regex 不可编译/空匹配）。

### 阶段 5：编译器（determinism + 语义校验）

规范：
- `SPEC_DETERMINISM.md`
- `SPEC_PROFILE_YAML.md`（兜底 MATCH 要求）

DoD：
- 订阅合并顺序、去重 key、命名冲突后缀、排序规则都按规范
- 策略组命名空间校验（组名唯一且不与节点冲突）
- `@all` 展开与 `url-test` 筛选顺序稳定
- 最终必须有兜底 `MATCH,<ACTION>`，否则报错

建议测试：
- 输入抖动（订阅顺序变化）下输出仍稳定（golden）。
- 同一输入多次编译输出字节级一致。

### 阶段 6：渲染器（先输出三段 block，不碰模板）

规范：
- `SPEC_RENDER_TARGETS.md`

DoD：
- 能从 IR 生成 `proxiesBlock/groupsBlock/rulesBlock`
- Surge/Shadowrocket 的名称可表示性规则生效（逗号/引号/双引号报错）
- `MATCH -> FINAL`（Surge/Shadowrocket）

建议测试：
- 直接对 block 做 golden（不用模板）。

### 阶段 7：模板注入（锚点/section/managed-config）

规范：
- `SPEC_TEMPLATE_ANCHORS.md`
- `SPEC_DETERMINISM.md`（managed-config URL 序列化）

DoD：
- 锚点缺失/重复/不独占一行都报错
- Shadowrocket/Surge section 校验报错可定位
- 缩进继承与 CRLF/LF 继承正确
- Surge `#!MANAGED-CONFIG`：插入/重写 URL 规则正确（且稳定）

### 阶段 8：HTTP API（端到端串起来）

规范：
- `SPEC_HTTP_API.md`

DoD：
- GET `/sub` 与 POST `/api/convert` 行为一致
- 成功响应 `text/plain`；失败响应 `application/json`
- 任何阶段错误都能映射成规范错误结构（含 stage）

### 阶段 9：端到端 golden tests（用 materials 做总验收）

规范：
- `docs/materials/README.md`

DoD：
- 使用 `docs/materials/` 的 profile/template/ruleset/subscription，三种 target 都能生成稳定输出
- 关键边界：Surge managed-config URL 必须按规范序列化（GET/POST 等价）

---

## 3. 可回溯：变更必须“有证据链”

任何行为变更都必须满足：
1) **先改规范**（SPEC 或 DESIGN，说明行为/原因）  
2) **再改实现**  
3) **补/改测试**（单测或 golden）证明变更是刻意的、可验证的  

建议维护一个“决策记录”（为什么这样做、有哪些替代、取舍是什么）：
- `DECISIONS.md`

---

## 4. 你应该坚持的底线

- 严格模式：不引入“容错猜测”（遇到问题直接报错，用户修远程文件）
- 数据结构优先：IR 只表达 Proxy/Group/Rule，不把全局配置塞进 IR
- 输出可复现：稳定性规则必须可测（不是“看起来差不多”）

