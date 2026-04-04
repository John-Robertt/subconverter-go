# Profile YAML 规范（v1）

本文档定义 profile（配置描述文件）的语法与语义。profile 的目标是：描述如何把“订阅节点”编译为目标客户端配置，并在需要时为订阅节点附加链式代理出口。

本项目没有“宽松修复”开关：profile 任何语法或语义错误都必须导致 HTTP 返回错误。

---

## 1. 顶层结构

profile 是一个 YAML 文档，顶层是 map（键值对），推荐最小结构如下：

```yaml
version: 1

template:
  clash: "https://example.com/base_clash.yaml"
  surge: "https://example.com/base_surge.conf"

public_base_url: "https://sub-api.example.com/sub"

custom_proxy:
  - name: EXIT
    type: socks5
    server: 1.2.3.4
    port: 1080
    username: user
    password: pass

custom_proxy_group:
  - "PROXY`select`[]AUTO[]DIRECT"
  - "AUTO`url-test`(HK|SG)`http://www.gstatic.com/generate_204`300"

proxy_chain:
  - type: all
    via: EXIT

ruleset:
  - "DIRECT,https://example.com/LAN.list"
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
- 支持的 key：`clash`、`shadowrocket`、`surge`、`quanx`（v1）
- value：模板 URL（字符串）

约束：
- `mode=config` 时必须存在，并且必须包含目标 `target` 对应的模板 URL。
- URL 必须是 `http` 或 `https`。

### 2.3 `public_base_url`（可选，但 `target=surge` 时强烈建议）

- 类型：字符串（URL）
- 约束：
  - 必须是 `http` 或 `https` 的绝对 URL
  - 不得包含 query 或 fragment（即不允许 `?` 与 `#`）
  - 推荐直接指向本服务的 `GET /sub` 端点（含路径，不含 query），例如：`https://sub-api.example.com/sub`
- 语义：
  - 用于生成 Surge 输出中的 `#!MANAGED-CONFIG <CURRENT_CONVERT_URL> ...` 行。
  - 若提供该字段，服务端生成 `<CURRENT_CONVERT_URL>` 时必须以它作为 base URL，而不是从当前请求的 Host 或反代头推导。

### 2.4 `custom_proxy`（可选）

- 类型：list[object]
- 语义：定义链式代理中的出口代理（下文简称 `EXIT`）。

约束：
- `custom_proxy.name` 必须非空、全局唯一。
- `custom_proxy.name` 不得为保留名 `DIRECT` / `REJECT`。
- `custom_proxy.name` 不得与策略组名冲突。
- `custom_proxy` 不属于订阅输入，不参与订阅去重。
- `custom_proxy` 不参与 `custom_proxy_group` 的成员选择池。
- `custom_proxy` 仅用于 `proxy_chain.via` 引用；未被任何生效的 `proxy_chain` 规则引用的 `custom_proxy` 可以不进入最终输出。

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

关于稳定性：
- `@all` 表示“全部订阅节点”，不包含 `custom_proxy`。
- 正则筛选的对象是订阅节点的 `MatchName`，见《输出稳定性与规范化规范》。

### 2.6 `proxy_chain`（可选）

- 类型：list[object]
- 语义：把一组订阅节点绑定到单个 `EXIT`，形成“订阅节点 -> EXIT”的单向链式代理。

字段：
- `type`：`all` | `regex` | `group`
- `via`：`custom_proxy.name`
- `pattern`：当 `type=regex` 时必填
- `group`：当 `type=group` 时必填

示例：

```yaml
proxy_chain:
  - type: all
    via: EXIT

  - type: regex
    pattern: "(HK|SG)"
    via: EXIT

  - type: group
    group: PROXY
    via: EXIT
```

约束：
- `via` 必须引用已定义的 `custom_proxy.name`。
- `type=all`：命中全部订阅节点。
- `type=regex`：按订阅节点 `MatchName` 做 Go RE2 正则匹配。
- `type=group`：引用已定义策略组，并将其成员递归展开为最终订阅节点集合；`DIRECT` / `REJECT` 不属于展开结果。
- `type=regex` 或 `type=group` 的选择结果不能为空；否则必须报错。
- 同一订阅节点若被多个 `proxy_chain` 规则命中：
  - 若 `via` 相同：允许，结果等价于一次赋值。
  - 若 `via` 不同：必须报错（链路冲突）。
- `proxy_chain` 当前仅对 `target=clash|surge` 生效；若目标不支持该特性，服务端必须返回错误。

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
  - Clash（mihomo）：服务端不拉取、不解析 ruleset 内容，而是把每个 URL 渲染为一个 `rule-providers` 条目，并在 `rules:` 中输出 `RULE-SET,<PROVIDER_NAME>,<ACTION>`
  - Surge / Shadowrocket：不展开 ruleset 内容；最终配置中输出远程引用行（例如 `RULE-SET,<URL>,<ACTION>`），由客户端自行拉取
  - Quantumult X：不展开 ruleset 内容；最终配置中输出到 `[filter_remote]` 的远程引用行，并通过 `force-policy` 绑定策略组

约束（v1）：
- `ACTION` 必须是 `DIRECT` / `REJECT` 或已定义的策略组名；否则必须报错。

### 2.8 `rule`（可选，但强烈推荐）

- 类型：list[string]
- 语义：追加 inline 规则（按顺序）。

每一项的语法：Clash classical 规则行（见 `SPEC_RULES_CLASH_CLASSICAL.md`）。

约束（v1 强制）：
- 最终规则列表必须包含兜底规则 `MATCH,<ACTION>`。如果 profile 没有提供，服务端必须返回错误。

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

1. 显式成员列表：

```
<GROUP_NAME>`select`[]<MEMBER_1>[]<MEMBER_2>...
```

2. 正则筛选订阅节点：

```
<GROUP_NAME>`select`<REGEX>
```

说明：
- `<GROUP_NAME>`：策略组名（非空）
- `<MEMBER_n>`：允许：
  - 其他组名
  - 内置 action：`DIRECT`、`REJECT`
  - 特殊 token：`@all`（表示“所有订阅节点”，由编译器在编译阶段展开）
- `<REGEX>`：Go RE2 正则；用于从订阅节点 `MatchName` 中筛选成员

约束：
- 至少要有 1 个成员。
- 使用 `<REGEX>` 写法时，筛选结果不能为空。
- `custom_proxy` 不得作为显式成员或正则筛选对象。

### 4.2 `url-test` 组

语法：

```
<GROUP_NAME>`url-test`<REGEX>`<URL>`<INTERVAL_SEC>[`<TOLERANCE_MS>]
```

说明：
- `<REGEX>`：Go RE2 正则，用于从订阅节点 `MatchName` 中筛选成员
- `<URL>`：测试 URL（http/https）
- `<INTERVAL_SEC>`：整数，单位秒
- `<TOLERANCE_MS>`：可选，整数，单位毫秒

约束：
- `<REGEX>` 必须可编译；编译失败即错误。
- 筛选结果不能为空。
- `custom_proxy` 不属于筛选对象。

---

## 5. 规则输入：Clash classical（统一语法）

v1 将 inline rule（profile 的 `rule`）统一解析为 Clash classical 规则行。

`ruleset` 指向的远程文件内容在 v1 默认不由服务端解析，仅作为 URL 引用写入最终配置，由客户端自行拉取。

完整的规则语法、支持的类型子集、以及必须报错的边界条件，统一定义在：
- `SPEC_RULES_CLASH_CLASSICAL.md`

---

## 6. 校验规则（必须报错的情况）

profile 解析或编译阶段至少必须校验：
- `version` 缺失或不为 1
- `template` 缺失、`target` 模板缺失、模板 URL 非法
- `custom_proxy` 字段缺失、类型不支持、名称冲突、端口非法、必填字段缺失
- `custom_proxy_group` 指令语法错误、类型不支持、引用不存在、`url-test` regex 非法或匹配为空
- `proxy_chain` 语法错误、`via` 不存在、group 引用不存在、选择结果为空、链路冲突
- `ruleset` 行语法错误、URL 非法
- `rule` 行语法错误
- 最终规则缺少兜底 `MATCH,<ACTION>`

---

## 7. 错误定位要求（面向用户修远程文件）

服务端返回错误时应尽可能包含：
- 出错阶段（stage）：`parse_profile` / `compile` 等
- 远程 URL（如果来自远程资源）
- 行号（如果适用）
- 片段（snippet）：原始出错片段（截断到合理长度）

错误响应 JSON 结构在《HTTP API 规范》中定义。
