# Profile YAML 规范（v1）

本文档定义 profile（配置描述文件）的**语法与语义**。profile 的目标是：用尽可能接近 Rules.ini 的“指令列表”写法，描述如何把“订阅节点”编译为目标客户端的配置文件。

本项目采用“严格模式”（唯一模式）：profile 任何语法/语义错误都必须导致 HTTP 返回错误。

---

## 1. 顶层结构

profile 是一个 YAML 文档，顶层是 map（键值对），推荐最小结构如下：

```yaml
version: 1

template:
  clash: "https://example.com/base_clash.yaml"
  shadowrocket: "https://example.com/base_sr.conf"
  surge: "https://example.com/base_surge.conf"

public_base_url: "https://sub-api.example.com/sub"

custom_proxy_group:
  - "PROXY`select`[]AUTO[]DIRECT"
  - "AUTO`url-test`(HK|SG)`http://www.gstatic.com/generate_204`300"

ruleset:
  - "DIRECT,https://example.com/LAN.list"
  - "REJECT,https://example.com/BanAD.list"
  - "PROXY,https://example.com/Proxy.list"

rule:
  - "MATCH,PROXY"
```

---

## 2. 字段定义

### 2.1 `version`（必填）

- 类型：整数
- 约束：必须为 `1`
- 语义：profile 规范版本，用于未来兼容。

### 2.2 `template`（必填）

- 类型：map
- 支持的 key：`clash`、`shadowrocket`、`surge`（v1）
- value：模板的 URL（字符串）

约束：
- `mode=config` 时必须存在，并且必须包含目标 `target` 对应的模板 URL。
- URL 必须是 `http` 或 `https`。

### 2.3 `public_base_url`（可选，但 `target=surge` 时强烈建议）

- 类型：字符串（URL）
- 约束：
  - 必须是 `http` 或 `https` 的绝对 URL
  - 不得包含 query/fragment（即不允许 `?` 与 `#`）
  - 推荐直接指向本服务的 `GET /sub` 端点（含路径，不含 query），例如：`https://sub-api.example.com/sub`
- 语义：
  - 用于生成 Surge 输出中的 `#!MANAGED-CONFIG <CURRENT_CONVERT_URL> ...` 行。
  - 若提供该字段，服务端生成 `<CURRENT_CONVERT_URL>` 时必须以它作为 base URL，而不是从当前请求的 Host/反代头推导。

### 2.4 `custom_proxy_group`（可选，推荐）

- 类型：list[string]
- 语义：按顺序定义策略组（组会被编译进 Core IR，再由各 target renderer 生成对应语法）。

注意：本字段的每一项是“指令字符串”，语法与 Rules.ini 的 `custom_proxy_group=` 类似，但 v1 只支持一个**严格子集**（见第 3 节）。
关于策略组/成员的最终排序、`@all` 展开顺序等稳定性要求，见《输出稳定性与规范化规范》。

### 2.5 `ruleset`（可选）

- 类型：list[string]
- 语义：按顺序引入远程规则集，并绑定默认 ACTION。

每一项的语法：

```
ACTION,URL
```

其中：
- `ACTION`：策略名（例如 `DIRECT`、`REJECT`、`PROXY`，或任意策略组名）
- `URL`：远程规则集 URL（http/https）

语义：
- 服务端会拉取 URL 内容并解析为规则列表，然后在最终规则输出中按 `ruleset` 的顺序展开插入。
- 规则集文件内部每行按“Clash classical 规则行”解析（见 `SPEC_RULES_CLASH_CLASSICAL.md`）。如果规则行缺省 ACTION，则使用该 `ruleset` 指令指定的 `ACTION` 作为默认值。

### 2.6 `rule`（可选，但强烈推荐）

- 类型：list[string]
- 语义：追加 inline 规则（按顺序）。

每一项的语法：Clash classical 规则行（见 `SPEC_RULES_CLASH_CLASSICAL.md`）。

约束（v1 强制）：
- 最终规则列表必须包含兜底规则（`MATCH,<ACTION>`）。如果 profile 没有提供，服务端必须返回错误（避免生成“无兜底”的配置）。

---

## 3. `custom_proxy_group` 指令语法（v1 子集）

v1 仅支持以下两种组类型：`select`、`url-test`。

### 3.1 `select` 组

语法：

```
<GROUP_NAME>`select`[]<MEMBER_1>[]<MEMBER_2>...
```

说明：
- `<GROUP_NAME>`：策略组名（非空）
- `<MEMBER_n>`：成员名，允许：
  - 其他组名（引用必须存在，否则错误）
  - 内置 action：`DIRECT`、`REJECT`
  - 特殊 token：`@all`（表示“所有订阅节点”，由编译器在编译阶段展开）

约束：
- 至少要有 1 个成员（`[]...`）。

示例：

```yaml
custom_proxy_group:
  - "PROXY`select`[]@all[]DIRECT"
```

### 3.2 `url-test` 组

语法：

```
<GROUP_NAME>`url-test`<REGEX>`<URL>`<INTERVAL_SEC>[`<TOLERANCE_MS>]
```

说明：
- `<REGEX>`：Go RE2 正则，用于从订阅节点的 `Proxy.Name` 中筛选成员
- `<URL>`：测试 URL（http/https）
- `<INTERVAL_SEC>`：整数，单位秒
- `<TOLERANCE_MS>`：可选，整数，单位毫秒（未提供则使用 renderer 的默认值）

约束：
- `<REGEX>` 必须可编译；编译失败即错误。
- 筛选结果不能为空（否则错误；避免生成“空组”）。

示例：

```yaml
custom_proxy_group:
  - "AUTO`url-test`(HK|SG)`http://www.gstatic.com/generate_204`300`50"
```

---

## 4. 规则输入：Clash classical（统一语法）

v1 将规则输入统一为 Clash classical 规则行：
- profile 的 `rule`（inline rule）
- profile 的 `ruleset` 拉取到的远程文件内容（ruleset file）

完整的规则语法、支持的类型子集、ruleset 中 ACTION 缺省、以及必须报错的边界条件，统一定义在：
- `SPEC_RULES_CLASH_CLASSICAL.md`

---

## 5. 校验规则（必须报错的情况）

profile 解析/编译阶段至少必须校验：
- `version` 缺失或不为 1
- `template` 缺失、`target` 模板缺失、模板 URL 非法
- `custom_proxy_group` 指令语法错误、类型不支持、引用不存在、`url-test` regex 非法/匹配为空
- `ruleset` 行语法错误、URL 非法
- `rule` 行语法错误
- 最终规则缺少兜底 `MATCH,<ACTION>`

---

## 6. 错误定位要求（面向用户修远程文件）

服务端返回错误时应尽可能包含：
- 出错阶段（stage）：`parse_profile` / `parse_ruleset` / `compile` 等
- 远程 URL（如果来自远程资源）
- 行号（如果是文本行错误）
- 片段（snippet）：原始出错行（截断到合理长度）

错误响应 JSON 结构在《HTTP API 规范》中定义。
