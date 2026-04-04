# 渲染规范（v1）：Clash / Surge / Shadowrocket / Quantumult X

本文档定义：编译阶段产出的“核心中间态”（IR：Proxies/Groups/Rules）如何渲染为各目标客户端可导入的配置文本。

原则：
- 只覆盖 v1 承诺的最小能力集（SS 订阅节点、链式派生节点、`select/url-test` 策略组、Clash classical 规则）。
- 目标差异通过“渲染阶段”吸收：同一份 profile 可以生成不同客户端配置。
- 模板只提供骨架；服务端生成文本块并注入锚点。

---

## 1. 输入：核心中间态（IR）假设

渲染器接收编译后的结构化数据：
- `Proxies[]`：包含两类节点：
  - 原始订阅节点：`type=ss`
  - 链式派生节点：`type=ss/http/https/socks5/socks5-tls`
- 每个 Proxy 都具备：
  - 内部唯一标识 `proxyID`
  - 最终展示名 `Name`
  - 若为链式派生节点，还具备 `ViaProxyID`，并且该引用必须指向同一输出中的原始订阅节点
- `Groups[]`：仅包含 `type=select|url-test`，成员为类型化引用：
  - `proxy`：引用某个 `proxyID`
  - `group`：引用某个策略组名
  - `builtin`：引用 `DIRECT` / `REJECT`
- `Rules[]`：仅包含 v1 规则类型
- `RulesetRefs[]`：profile 中 `ruleset` 的远程引用信息（`ACTION,URL`）

并且应已满足《输出稳定性与规范化规范》：
- 原始订阅节点已去重，`proxyID` 已稳定生成，展示名已唯一化，顺序稳定
- 派生节点已稳定生成，命名与顺序稳定
- 自动诊断组已追加到 `Groups[]`

---

## 2. 输出：注入块（通用）

所有 target 都必须生成三段文本块：
- `proxiesBlock`
- `groupsBlock`
- `rulesBlock`

并且当 `target=clash` 时，还必须额外生成：
- `ruleProvidersBlock`

并且当 `target=quanx` 时，还必须额外生成：
- `rulesetsBlock`

然后交由模板注入器替换：
- `#@PROXIES@#`
- `#@GROUPS@#`
- `#@RULE_PROVIDERS@#`（仅 Clash）
- `#@RULESETS@#`（仅 QuanX）
- `#@RULES@#`

---

## 3. 通用渲染约束

### 3.1 `proxyID` 回查

当组成员是 `proxy` 引用时，renderer 必须先通过 `proxyID` 查到该节点的最终名称表示，再写入目标配置。

### 3.2 链式派生节点引用约束

当某个节点存在 `ViaProxyID` 时：
- `ViaProxyID` 必须能在同一份输出中解析到一个原始订阅节点
- 不允许指向另一个派生节点
- 若引用不存在或类型不合法，必须报错（建议错误码 `INVALID_ARGUMENT` 或 `CHAIN_PROXY_NOT_FOUND`）

### 3.3 自动诊断组

自动诊断组与普通策略组的渲染规则完全一致；它只是编译器追加的组，不是新的目标语法。

---

## 4. 目标：Clash（YAML）

### 4.1 proxiesBlock（YAML sequence items）

每个节点输出为一个 YAML mapping（作为 list item）。

#### 4.1.1 原始订阅 SS 节点

字段最小集合：
- `name`
- `type: ss`
- `server`
- `port`
- `cipher`
- `password`

可选字段（v1）：
- `plugin` / `plugin-opts`

#### 4.1.2 链式派生节点

支持以下类型：
- `ss`
- `http`
- `https`
- `socks5`
- `socks5-tls`

对应语义：
- `type=ss`：沿用 SS 节点渲染语法
- `type=http|https`：输出 Clash `http` 代理；`https` 额外输出 `tls: true`
- `type=socks5|socks5-tls`：输出 Clash `socks5` 代理；`socks5-tls` 额外输出 `tls: true`
- 若存在用户名/密码，输出 `username` / `password`

链式字段：
- 当节点存在 `ViaProxyID` 时，必须追加：

```yaml
dialer-proxy: <SUB_PROXY_NAME>
```

其中 `<SUB_PROXY_NAME>` 必须是被引用原始订阅节点的最终展示名。

### 4.2 groupsBlock（YAML sequence items）

每个策略组输出为一个 YAML mapping：
- `name`
- `type`
- `proxies`

`url-test` 额外字段：
- `url`
- `interval`
- `tolerance`（可选）

成员可为：
- 节点名
- 其他组名
- `DIRECT` / `REJECT`

说明：
- 正则组选中链式派生节点后，输出语法与普通节点相同。
- 自动诊断组也按同样规则输出。

### 4.3 ruleProvidersBlock

当 profile 提供 `ruleset` 时，Clash 输出不展开 ruleset 内容，而是生成 `rule-providers:` 配置。

每个 ruleset URL 必须生成一个 provider 条目，字段最小集合：
- `type: http`
- `behavior: classical`
- `url: "<RULESET_URL>"`
- `interval: 86400`
- `format: text`

当 profile 未提供任何 `ruleset` 时：
- `ruleProvidersBlock` 必须输出一个空 map（例如 `{}`）。

### 4.4 rulesBlock

Clash 的 `rules:` 输出顺序固定为：
1. ruleset 引用行：`RULE-SET,<PROVIDER_NAME>,<ACTION>`
2. inline 规则：`TYPE,VALUE,ACTION[,no-resolve]`

---

## 5. 目标：Surge（INI-like）

### 5.1 顶部 `#!MANAGED-CONFIG`

当 `mode=config&target=surge`：
- 输出的第一个非空行必须是 `#!MANAGED-CONFIG <CURRENT_CONVERT_URL> ...`

### 5.2 proxiesBlock（写入 `[Proxy]` 段）

#### 5.2.1 内置项

Surge 不允许在 `[Proxy]` 段中定义内部策略名，因此不得输出 `DIRECT = ...` / `REJECT = ...`。

#### 5.2.2 原始订阅 SS 节点

每个 SS 节点输出为一行：

```
<NAME> = ss, <SERVER>, <PORT>, encrypt-method=<CIPHER>, password=<PASSWORD>[, <EXTRA_KV>...]
```

#### 5.2.3 链式派生节点

支持以下最小语法：

```
<NAME> = ss, <SERVER>, <PORT>, encrypt-method=<CIPHER>, password=<PASSWORD>[, ...][, underlying-proxy=<SUB_PROXY_NAME>]
<NAME> = http, <SERVER>, <PORT>[, <USERNAME>, <PASSWORD>][, underlying-proxy=<SUB_PROXY_NAME>]
<NAME> = https, <SERVER>, <PORT>[, <USERNAME>, <PASSWORD>][, underlying-proxy=<SUB_PROXY_NAME>]
<NAME> = socks5, <SERVER>, <PORT>[, <USERNAME>, <PASSWORD>][, underlying-proxy=<SUB_PROXY_NAME>]
<NAME> = socks5-tls, <SERVER>, <PORT>[, <USERNAME>, <PASSWORD>][, underlying-proxy=<SUB_PROXY_NAME>]
```

约束：
- 当节点存在 `ViaProxyID` 时，必须追加 `underlying-proxy=<SUB_PROXY_NAME>`。
- `<SUB_PROXY_NAME>` 必须引用同一份输出中的原始订阅节点最终名称表示。

#### 5.2.4 名称可表示性

由于 Surge 使用 `NAME = ...` 与逗号分隔成员列表：
- 策略组名与规则 action 不得包含 `,` 或 `=` 或控制字符；否则必须报错。
- 节点名若包含 `,`，必须用双引号包裹。
- 节点名若包含 `"` 则必须报错。

### 5.3 groupsBlock（写入 `[Proxy Group]` 段）

#### 5.3.1 select

```
<GROUP> = select, <MEMBER_1>, <MEMBER_2>, ...
```

#### 5.3.2 url-test

```
<GROUP> = url-test, <MEMBER_1>, <MEMBER_2>, ..., url=<URL>, interval=<SEC>[, tolerance=<MS>]
```

成员允许：
- 节点名
- 其他组名
- `DIRECT` / `REJECT`

### 5.4 rulesBlock（写入 `[Rule]` 段）

输出顺序固定为：
1. ruleset 远程引用行：`RULE-SET,<URL>,<ACTION>`
2. inline 规则：`TYPE,VALUE,ACTION[,no-resolve]`

规则类型映射：
- `MATCH` -> `FINAL`
- 其它类型原样输出

---

## 6. 目标：Shadowrocket（INI-like）

v1 规定 Shadowrocket 的基础渲染语法与 Surge 相同（使用 `[Proxy]` / `[Proxy Group]` / `[Rule]` 三段）。

差异点：
- 不要求输出 `#!MANAGED-CONFIG`
- 若最终 `Proxies[]` 中存在任何链式派生节点（即存在 `ViaProxyID != ""` 的节点），必须返回错误：
  - `code: UNSUPPORTED_TARGET_FEATURE`
  - `stage: render`
  - `message`: `target=shadowrocket 当前不支持 proxy_chain`

---

## 7. 目标：Quantumult X（INI-like）

### 7.1 proxiesBlock（写入 `[server_local]` 段）

每个 SS 节点输出为一行：

```
shadowsocks = <SERVER>:<PORT>, method=<CIPHER>, password=<PASSWORD>, tag=<NAME>
```

名称可表示性：
- 节点 tag 若包含 `,`，必须用双引号包裹
- 节点名若包含 `"` 则必须报错

### 7.2 groupsBlock（写入 `[policy]` 段）

#### 7.2.1 select -> `static`

```
static=<GROUP>, <MEMBER_1>, <MEMBER_2>, ...
```

#### 7.2.2 url-test -> `url-latency-benchmark`

```
url-latency-benchmark=<GROUP>, <MEMBER_1>, <MEMBER_2>, ..., check-interval=<SEC>[, tolerance=<MS>]
```

内置 action 映射：
- `DIRECT` -> `direct`
- `REJECT` -> `reject`

### 7.3 rulesetsBlock（写入 `[filter_remote]` 段）

每个 ruleset 输出为一行：

```
<URL>, tag=<TAG>, force-policy=<POLICY>, enabled=true
```

### 7.4 rulesBlock（写入 `[filter_local]` 段）

规则行语法：

```
TYPE,VALUE,ACTION[,no-resolve]
```

映射约束：
- `MATCH` -> `FINAL`
- `IP-CIDR6` -> `IP6-CIDR`
- `DIRECT` -> `direct`
- `REJECT` -> `reject`

### 7.5 链式代理限制

若最终 `Proxies[]` 中存在任何链式派生节点，必须返回错误：
- `code: UNSUPPORTED_TARGET_FEATURE`
- `stage: render`
- `message`: `target=quanx 当前不支持 proxy_chain`
