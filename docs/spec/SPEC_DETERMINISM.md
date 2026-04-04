# 输出稳定性与规范化规范（v1）

本文档定义“同一组输入 -> 同一份输出”的稳定性要求，以及订阅节点的去重、命名、排序和链式代理编译规则。

目标：让生成结果可 diff、可回滚、可定位问题；避免因为上游顺序抖动、节点重名或链路匹配歧义导致输出噪音。

---

## 1. 名词与约束

### 1.1 内部身份与展示名称分离

编译器不得使用“节点名称字符串”作为内部主键。v1 定义四个不同概念：

- `ProxyID`
  - 编译期内部唯一标识，仅在当前请求内使用，不对外暴露
  - 用于 group 展开、chain 命中、冲突检测、渲染前引用解析
- `SemanticKey`
  - 仅用于判定两个订阅节点是否语义相同，供去重使用
  - `custom_proxy` 不参与该去重
- `MatchName`
  - 用于正则匹配的节点名
  - 基于原始节点名或 `server:port` 生成
  - 不追加 `-2/-3` 等去重后缀
- `DisplayName`
  - 最终渲染到输出配置中的名称
  - 必须在最终命名空间内唯一

约束：
- 编译期一律使用 `ProxyID` 做身份识别。
- `regex` 选择器与正则策略组匹配对象都是订阅节点的 `MatchName`，不是最终 `DisplayName`。
- 最终输出使用 `DisplayName`。

### 1.2 命名空间

在 Clash、Surge 等目标中，节点名、出口代理名和策略组名最终都会以字符串形式被引用。

因此 v1 约束：
- 策略组名必须全局唯一。
- `custom_proxy.name` 必须全局唯一。
- `custom_proxy.name` 不得与任何策略组名冲突。
- 最终 `DisplayName` 不得与任何策略组名或 `custom_proxy.name` 冲突；若冲突，编译器只能调整订阅节点的 `DisplayName`，不得静默改写策略组名或 `custom_proxy.name`。

### 1.3 保留名

以下字符串作为内置 action 保留（大小写敏感，v1 以大写为准）：
- `DIRECT`
- `REJECT`

约束：
- 策略组名不得为保留名。
- `custom_proxy.name` 不得为保留名。
- 订阅节点若为保留名，允许编译器进行确定性重命名，以保证输出可用。

---

## 2. 订阅合并顺序（稳定输入序）

当请求包含多个订阅 URL：
- 合并顺序：按请求参数顺序合并（GET 的 `sub=` 出现顺序；POST 的 `subs[]` 数组顺序）。
- 每个订阅内部：按文本行顺序解析得到 Proxy 列表。

该“合并顺序”是后续去重、命名冲突处理和 regex 选择器匹配结果顺序的稳定基准。

---

## 3. 订阅节点规范化（Normalization）

### 3.1 字段规范化

对每个订阅 Proxy，v1 至少应做以下规范化（不改变语义）：
- 原始名称：去首尾空白；不得包含 `\r`、`\n`、`\0`
- `Server`：去首尾空白；域名转小写（IP 原样）
- `Cipher`：去首尾空白并转小写
- `Password`：去首尾空白
- `Plugin` / `PluginOpts`：去首尾空白，不做重排

### 3.2 `MatchName` 生成

对每个订阅 Proxy 先得到一个基础匹配名：
- 若订阅提供 `#name`：URL 解码后得到的名称，去首尾空白，作为基础匹配名
- 否则：使用 `"<server>:<port>"` 作为基础匹配名

基础匹配名必须做最小兼容性规范化：
- 将所有 `=` 字符替换为 `-`

规范化后的结果即 `MatchName`。

说明：
- `MatchName` 仅用于正则匹配与调试定位。
- `MatchName` 不要求全局唯一。

---

## 4. 去重（Deduplication）

v1 需要去重以避免重复订阅节点污染策略组与 UI。去重必须确定且可复现。

### 4.1 去重范围

- 仅订阅节点参与去重。
- `custom_proxy` 不参与去重；即使多个 `custom_proxy` 的连接参数完全相同，也按不同出口保留。

### 4.2 去重 key（SS）

对订阅 SS 节点定义 `SemanticKey`，建议包含：
- `Type`
- `Server`
- `Port`
- `Cipher`
- `Password`
- `Plugin`
- `PluginOpts`

### 4.3 去重策略

- 保留第一次出现的订阅节点（按订阅合并顺序）。
- 后续重复 `SemanticKey` 的订阅节点直接丢弃。

注意：
- 去重发生在最终 `DisplayName` 分配之前。
- 去重不影响 `custom_proxy`。

---

## 5. `custom_proxy` 规范化

`custom_proxy` 的字段规范化遵循 profile 语义，但它不参与订阅去重与策略组筛选。

约束：
- 输出顺序以 profile 声明顺序为准。
- `custom_proxy.name` 作为固定锚点，不允许编译器自动重命名。
- 只有被生效的 `proxy_chain` 规则引用到的 `custom_proxy` 才需要进入最终输出。

---

## 6. `DisplayName` 分配与命名冲突处理

### 6.1 固定命名对象

以下名称视为固定锚点，编译器不得自动改写：
- `custom_proxy.name`
- 策略组名
- 保留名 `DIRECT` / `REJECT`

### 6.2 订阅节点 `DisplayName` 分配

对每个订阅节点按订阅合并顺序依次分配最终 `DisplayName`：
- 若 `MatchName` 未被固定命名对象或之前节点占用，且不是保留名：使用 `MatchName`
- 否则：追加后缀 `-2`、`-3`...，直到找到未占用的名字

该规则必须只依赖：
- 订阅合并顺序
- 已保留的固定命名对象集合

说明：
- 这保证即使多个订阅节点拥有相同 `MatchName`，内部仍可通过 `ProxyID` 区分，最终仅在渲染阶段通过 `DisplayName` 消歧。

---

## 7. 链式代理编译稳定性

### 7.1 `proxy_chain` 命中对象

- `all`：命中全部订阅节点的 `ProxyID`
- `regex`：按订阅节点 `MatchName` 做正则匹配，返回匹配到的 `ProxyID` 集合
- `group`：按策略组名递归展开为最终订阅节点 `ProxyID` 集合

约束：
- `group` 展开时若成员为 `DIRECT` / `REJECT`，忽略它们
- `group` 展开若出现组循环，必须报错
- 命中结果顺序必须与订阅合并顺序一致

### 7.2 冲突处理

对同一个订阅节点 `ProxyID`：
- 多条 `proxy_chain` 规则命中且 `via` 相同：允许
- 多条 `proxy_chain` 规则命中且 `via` 不同：必须报错

编译器内部应把链式代理关系保存为：
- `订阅节点 ProxyID -> custom_proxy ProxyID`

不得直接保存为名称字符串映射。

---

## 8. 输出顺序（Ordering）

### 8.1 代理输出顺序

最终 `proxies` 输出顺序固定为：
1. 被生效 `proxy_chain` 引用到的 `custom_proxy`，按 profile 声明顺序输出
2. 订阅节点，按订阅合并顺序输出

不做额外排序。

### 8.2 策略组输出顺序

- 策略组定义顺序必须与 profile 的 `custom_proxy_group` 列表顺序一致
- 策略组成员顺序：
  - `select`（显式成员列表）：按 profile 中 `[]成员` 的出现顺序输出；若出现 `@all`，则在该位置展开为“全部订阅节点”
  - `select`（正则筛选写法）：成员来自正则筛选，输出顺序为订阅合并顺序
  - `url-test`：成员来自正则筛选，输出顺序为订阅合并顺序

### 8.3 规则输出顺序

最终规则列表顺序必须严格为：
1. 按 profile 的 `ruleset` 列表顺序逐个插入
2. 再按 profile 的 `rule` 列表顺序追加 inline 规则

规则不做排序或去重。

---

## 9. `mode=list` 输出稳定性

当 `mode=list` 输出纯节点列表时：
- 仅输出订阅节点，不输出 `custom_proxy`
- 节点列表顺序：按 8.1 中的订阅节点顺序输出
- raw（明文）输出：每行输出一条 canonical 的 `ss://` URI；使用 `\n` 分行，并且末尾必须带一个 `\n`
- 当 `encode=base64`：对“raw 列表文本（UTF-8 字节序列）”做标准 base64 编码输出；不得换行折行

### 9.1 canonical `ss://` 行（v1）

v1 统一输出为 SIP002 常见形态（userinfo-base64）：

```
ss://<B64URL(method:password)>@<host>:<port>[/?plugin=<PCT_ENCODED(plugin)>][#<PCT_ENCODED(name)>]
```

规则：
- `name` 使用最终订阅节点 `DisplayName`
- 其它字段使用规范化后的 SS 字段

---

## 10. Surge `#!MANAGED-CONFIG` 的 URL 稳定性

当 `mode=config&target=surge` 时需要生成 `<CURRENT_CONVERT_URL>`。

为保证稳定，v1 要求：
- 无论请求是 GET 还是 POST，`<CURRENT_CONVERT_URL>` 都必须由服务端根据“解析后的参数”重新序列化生成，不得直接复用原始 query 字符串

- base URL 选择：
  1. 若 profile 提供 `public_base_url`，必须使用它作为 base URL
  2. 否则使用当前请求推导出的 base URL

- query 参数序列化顺序（固定）：
  1. `mode=config`
  2. `target=surge`
  3. `fileName=<name>`（可选）
  4. 按请求中订阅数组顺序重复输出 `sub=<url>`
  5. `profile=<url>`

该顺序只影响字符串稳定性，不影响语义。
