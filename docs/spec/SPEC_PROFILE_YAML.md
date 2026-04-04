# Profile YAML 规范（v1）

本文档定义 profile（配置描述文件）的语法与语义。profile 的目标是：用尽可能接近 Rules.ini 的“指令列表”写法，描述如何把“订阅节点”编译为目标客户端的配置文件，并在需要时为自定义代理生成链式派生节点。

本项目没有“宽松修复”开关：profile 任何语法/语义错误都必须导致 HTTP 返回错误。

---

## 1. 顶层结构

profile 是一个 YAML 文档，顶层是 map（键值对），推荐最小结构如下：

```yaml
version: 1

template:
  clash: "https://example.com/base_clash.yaml"
  shadowrocket: "https://example.com/base_sr.conf"
  surge: "https://example.com/base_surge.conf"
  quanx: "https://example.com/base_quanx.conf"

public_base_url: "https://sub-api.example.com/sub"

custom_proxy:
  - name: CORP-HTTP
    type: http
    server: proxy.example.com
    port: 8080
    username: user
    password: pass

custom_proxy_group:
  - "PROXY`select`[]AUTO[]@all[]DIRECT"
  - "AUTO`url-test`(HK|SG|US)`http://www.gstatic.com/generate_204`300`50"
  - "HTTP-CHAIN`select`(CORP-HTTP via)"

proxy_chain:
  - proxy: CORP-HTTP
    type: regex
    pattern: "(HK|SG)"

ruleset:
  - "DIRECT,https://example.com/LAN.list"
  - "REJECT,https://example.com/BanAD.list"
  - "PROXY,https://example.com/Proxy.list"

rule:
  - "MATCH,PROXY"
```

说明：
- `custom_proxy` 定义“基础代理模板”，不直接进入最终输出。
- `proxy_chain` 从订阅节点中选择一批节点，为某个 `custom_proxy` 生成一批“链式派生节点”。
- 每个 `custom_proxy` 还会自动生成一个诊断组：`CHAIN-<custom_proxy.name>`；该诊断组不需要用户手写到 `custom_proxy_group`。

---

## 2. 字段定义

### 2.1 `version`（必填）

- 类型：整数
- 约束：必须为 `1`
- 语义：profile 规范版本，用于未来兼容。

### 2.2 `template`（必填）

- 类型：map
- 支持的 key：`clash`、`shadowrocket`、`surge`、`quanx`（v1）
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

### 2.4 `custom_proxy`（可选）

- 类型：list[object]
- 语义：定义链式代理中的“基础代理模板”。

约束：
- `custom_proxy.name` 必须非空、全局唯一。
- `custom_proxy.name` 不得为保留名 `DIRECT` / `REJECT`。
- `custom_proxy.name` 不得与用户定义的策略组名冲突。
- `custom_proxy` 不属于订阅输入，不参与订阅去重。
- `custom_proxy` 不直接进入最终输出；只有被 `proxy_chain` 命中后生成的“派生节点”会进入最终输出。
- `custom_proxy` 本身不属于 `custom_proxy_group` 的显式成员池。

支持的 `type`：
- `ss`
- `http`
- `https`
- `socks5`
- `socks5-tls`

详细字段见第 3 节。

### 2.5 `custom_proxy_group`（可选，推荐）

- 类型：list[string]
- 语义：按顺序定义策略组（组会被编译进 Core IR，再由各 target renderer 生成对应语法）。

注意：本字段的每一项是“指令字符串”，语法与 Rules.ini 的 `custom_proxy_group=` 类似，但 v1 只支持一个明确子集（见第 4 节）。

关于选择池：
- 显式成员列表仍只支持“组名 / 内置动作 / `@all`”三类语义。
- 正则写法匹配“最终可输出节点”的展示名：既包含原始订阅节点，也包含链式派生节点。

关于保留命名空间：
- 用户定义的策略组名不得以 `CHAIN-` 开头；该前缀保留给自动诊断组。

### 2.6 `proxy_chain`（可选）

- 类型：list[object]
- 语义：从订阅节点中选择一批节点，为某个 `custom_proxy` 生成一批链式派生节点。

字段：
- `proxy`：`custom_proxy.name`
- `type`：`all` | `regex` | `group`
- `pattern`：当 `type=regex` 时必填
- `group`：当 `type=group` 时必填

示例：

```yaml
proxy_chain:
  - proxy: CORP-HTTP
    type: all

  - proxy: CORP-HTTP
    type: regex
    pattern: "(HK|SG)"

  - proxy: CORP-HTTP
    type: group
    group: PROXY
```

约束：
- `proxy` 必须引用已定义的 `custom_proxy.name`。
- `type=all`：命中全部原始订阅节点。
- `type=regex`：按原始订阅节点的最终展示名 `Proxy.Name` 做 Go RE2 正则匹配。
- `type=group`：引用已定义策略组，并将其成员递归展开为最终订阅节点集合；`DIRECT` / `REJECT` 不属于展开结果。
- `type=regex` 或 `type=group` 的选择结果不能为空；否则必须报错。
- 同一个 `custom_proxy` 可由多条 `proxy_chain` 规则命中；最终命中集合按并集去重。
- `proxy_chain` 当前仅对 `target=clash|surge` 生效；若目标不支持该特性，服务端必须返回错误。

补充说明：
- `proxy_chain` 的选择对象只看“原始订阅节点”。
- `proxy_chain type=group` 的递归展开只基于“用户定义策略组 -> 原始订阅节点”的关系，不把自动诊断组和派生节点重新纳入选择，避免形成自引用语义。

### 2.7 `ruleset`（可选）

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
- `ruleset` 用于“远程规则集引用”（ACTION 绑定 + 顺序控制），不同 target 的渲染策略不同：
  - Clash（mihomo）：服务端不拉取/不解析 ruleset 内容，而是把每个 URL 渲染为一个 `rule-providers` 条目，并在 `rules:` 中输出：
    - `RULE-SET,<PROVIDER_NAME>,<ACTION>`
  - Surge / Shadowrocket：不展开 ruleset 内容；最终配置中输出远程引用行（例如 `RULE-SET,<URL>,<ACTION>`），由客户端自行拉取。
  - Quantumult X：不展开 ruleset 内容；最终配置中输出到 `[filter_remote]` 的远程引用行，并通过 `force-policy` 绑定策略组。

约束（v1）：
- 无论 target 最终如何渲染，`ACTION` 必须是 `DIRECT/REJECT` 或已定义的策略组名；否则必须报错（引用不存在）。

### 2.8 `rule`（可选，但强烈推荐）

- 类型：list[string]
- 语义：追加 inline 规则（按顺序）。

每一项的语法：Clash classical 规则行（见 `SPEC_RULES_CLASH_CLASSICAL.md`）。

约束（v1 强制）：
- 最终规则列表必须包含兜底规则（`MATCH,<ACTION>`）。如果 profile 没有提供，服务端必须返回错误（避免生成“无兜底”的配置）。

---

## 3. `custom_proxy` 对象语法（v1）

### 3.1 通用字段

所有 `custom_proxy` 都支持以下通用字段：
- `name`：字符串，必填
- `type`：字符串，必填
- `server`：字符串，必填
- `port`：整数，必填，范围 `1..65535`
- `username`：字符串，可选
- `password`：字符串，可选；对 `ss` 为必填

约束：
- `name` 不得包含控制字符。
- `server` 不得为空。
- `port` 必须为合法端口。

### 3.2 `type=ss`

必填字段：
- `cipher`
- `password`

可选字段：
- `plugin`
- `plugin_opts`

说明：
- `plugin` / `plugin_opts` 的支持矩阵与目标渲染规范一致。
- `custom_proxy.type=ss` 的字段语义与订阅 SS 节点保持一致，但它来自 profile，而不是订阅文本。

示例：

```yaml
custom_proxy:
  - name: EXIT-SS
    type: ss
    server: exit.example.com
    port: 8388
    cipher: aes-128-gcm
    password: pass
```

### 3.3 `type=http|https|socks5|socks5-tls`

必填字段：
- `server`
- `port`

可选字段：
- `username`
- `password`

说明：
- `username` / `password` 要么都省略，要么按目标客户端允许的语法一起输出。

示例：

```yaml
custom_proxy:
  - name: EXIT-SOCKS
    type: socks5
    server: 1.2.3.4
    port: 1080
    username: user
    password: pass
```

---

## 4. `custom_proxy_group` 指令语法（v1 子集）

v1 仅支持以下两种组类型：`select`、`url-test`。

### 4.1 `select` 组

语法（两种写法，二选一）：

1) 显式成员列表（推荐：可读性更强）：

```
<GROUP_NAME>`select`[]<MEMBER_1>[]<MEMBER_2>...
```

2) 正则筛选节点（兼容常见 Rules.ini 写法）：

```
<GROUP_NAME>`select`<REGEX>
```

说明：
- `<GROUP_NAME>`：策略组名（非空）
- `<MEMBER_n>`：成员名，允许：
  - 其他组名（引用必须存在，否则错误）
  - 内置 action：`DIRECT`、`REJECT`
  - 特殊 token：`@all`（表示“所有原始订阅节点”，由编译器在编译阶段展开）
- `<REGEX>`：Go RE2 正则；用于从最终可输出节点的展示名 `Proxy.Name` 中筛选成员（包含原始订阅节点与链式派生节点，不包含其它策略组与 DIRECT/REJECT）。

约束：
- 至少要有 1 个成员（`[]...`）。
- 显式成员列表当前聚焦“组名 / 内置动作 / `@all`”三类语义，不支持直接引用单个节点；若需要按节点选取，请使用正则写法。
- 使用 `<REGEX>` 写法时，筛选结果不能为空（否则错误；避免生成“空组”）。
- `@all` 在编译阶段展开为全部原始订阅节点的内部引用；展开顺序按《输出稳定性与规范化规范》。

### 4.2 `url-test` 组

语法：

```
<GROUP_NAME>`url-test`<REGEX>`<URL>`<INTERVAL_SEC>[`<TOLERANCE_MS>]
```

说明：
- `<REGEX>`：Go RE2 正则，用于从最终可输出节点的展示名 `Proxy.Name` 中筛选成员
- `<URL>`：测试 URL（http/https）
- `<INTERVAL_SEC>`：整数，单位秒
- `<TOLERANCE_MS>`：可选，整数，单位毫秒（未提供则使用 renderer 的默认值）

约束：
- `<REGEX>` 必须可编译；编译失败即错误。
- 筛选结果不能为空（否则错误；避免生成“空组”）。

---

## 5. 自动诊断组

当某个 `custom_proxy` 至少生成了 1 个链式派生节点时，编译器必须自动追加一个诊断组：

```
CHAIN-<custom_proxy.name>
```

语义：
- 组类型：`select`
- 成员：该 `custom_proxy` 生成的全部派生节点，顺序按《输出稳定性与规范化规范》

约束：
- `CHAIN-` 是保留前缀；用户定义的策略组名不得使用该前缀。
- 自动诊断组属于最终组集合的一部分，因此会进入规则 action 命名空间检查与目标配置输出。

---

## 6. 规则输入：Clash classical（统一语法）

v1 将 inline rule（profile 的 `rule`）统一解析为 Clash classical 规则行。

`ruleset` 指向的远程文件内容在 v1 默认不由服务端解析（仅作为 URL 引用写入最终配置，由客户端自行拉取）。为了让同一份 profile 尽可能兼容多个客户端，v1 推荐 ruleset 文件内容使用 Clash classical list 常见写法（每行 `TYPE,VALUE`，通常不带 ACTION）。

完整的规则语法、支持的类型子集、以及必须报错的边界条件，统一定义在：
- `SPEC_RULES_CLASH_CLASSICAL.md`

---

## 7. 校验规则（必须报错的情况）

profile 解析/编译阶段至少必须校验：
- `version` 缺失或不为 1
- `template` 缺失、`target` 模板缺失、模板 URL 非法
- `custom_proxy` 字段缺失、类型不支持、名称冲突、端口非法、必填字段缺失
- 用户定义策略组名使用保留前缀 `CHAIN-`
- `custom_proxy_group` 指令语法错误、类型不支持、引用不存在、`url-test` regex 非法/匹配为空
- `proxy_chain` 语法错误、`proxy` 不存在、group 引用不存在、选择结果为空、目标不支持该特性
- 自动诊断组名与最终节点名或用户定义组名冲突
- `ruleset` 行语法错误、URL 非法
- `rule` 行语法错误
- 最终规则缺少兜底 `MATCH,<ACTION>`

---

## 8. 错误定位要求（面向用户修远程文件）

服务端返回错误时应尽可能包含：
- 出错阶段（stage）：`parse_profile` / `compile` 等
- 远程 URL（如果来自远程资源）
- 行号（如果是文本行错误）
- 片段（snippet）：原始出错行（截断到合理长度）

错误响应 JSON 结构在《HTTP API 规范》中定义。
