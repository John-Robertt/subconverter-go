# 渲染规范（v1）：Clash / Surge / Shadowrocket

本文档定义：编译阶段产出的“核心中间态”（IR：Proxies/Groups/Rules）如何渲染为各目标客户端可导入的配置文本。

原则：
- 只覆盖 v1 承诺的最小能力集（SS 节点、`select/url-test` 策略组、Clash classical 规则）。
- 任何无法表达/不支持的能力必须报错（严格模式）。
- 模板只提供骨架；服务端只生成三段文本并注入锚点（见《模板锚点与注入规范》）。

本规范参考了本仓库内的 `subconverter/`（C 版本）的主流格式行为，但这里定义的是“可实现且稳定的子集”，不是照搬其全部细节/兼容性分支。

---

## 1. 输入：核心中间态（IR）假设

渲染器接收编译后的结构化数据：
- `Proxies[]`：仅包含 `type=ss`
- `Groups[]`：仅包含 `type=select|url-test`
- `Rules[]`：仅包含 v1 规则类型（`DOMAIN/DOMAIN-SUFFIX/DOMAIN-KEYWORD/IP-CIDR/GEOIP/MATCH`）

并且应已满足《输出稳定性与规范化规范》：
- 节点已去重、命名唯一、排序稳定
- `@all` 已展开为具体节点名（或编译器可在渲染时展开，但必须等价且稳定）

---

## 2. 输出：三段注入块（通用）

无论目标为何，渲染器都必须生成三段文本块：
- `proxiesBlock`
- `groupsBlock`
- `rulesBlock`

然后交由模板注入器替换：
- `#@PROXIES@#`
- `#@GROUPS@#`
- `#@RULES@#`

模板注入算法与锚点约束见《模板锚点与注入规范》。

---

## 3. 目标：Clash（YAML）

### 3.1 proxiesBlock（YAML sequence items）

每个 SS 节点输出为一个 YAML mapping（作为 list item），字段最小集合：
- `name`：节点名（字符串）
- `type`：固定为 `ss`
- `server`：服务器地址
- `port`：端口（整数）
- `cipher`：加密方法（字符串，小写）
- `password`：密码（字符串）

可选字段（v1）：
- `udp`：布尔；仅当 IR 指定 UDP 且目标支持时输出
- `tfo`：布尔；仅当 IR 指定 TFO 且目标支持时输出
- `plugin` / `plugin-opts`：仅当 SS 节点携带 plugin 且属于支持矩阵时输出

密码的 YAML 表达要求：
- 若 `password` 仅由数字组成，仍必须作为字符串输出（禁止被 YAML 解释为数字）。

#### SS plugin 支持矩阵（Clash）

v1 支持：
- `simple-obfs` / `obfs-local`：
  - `plugin: obfs`
  - `plugin-opts.mode`：来自 `plugin` 参数中的 `obfs=...`
  - `plugin-opts.host`：来自 `plugin` 参数中的 `obfs-host=...`（可空）

其它 plugin：必须报错（错误码建议 `UNSUPPORTED_PLUGIN`），而不是静默丢弃。

### 3.2 groupsBlock（YAML sequence items）

每个策略组输出为一个 YAML mapping（作为 list item），字段最小集合：
- `name`：组名（字符串）
- `type`：`select` 或 `url-test`
- `proxies`：成员列表（字符串数组），元素可为：
  - 节点名
  - 其他组名
  - 内置 action：`DIRECT` / `REJECT`

`url-test` 额外字段：
- `url`：测试 URL
- `interval`：秒（整数）
- `tolerance`：毫秒（可选，整数）

### 3.3 rulesBlock（YAML sequence of strings）

每条规则输出为一个 YAML 字符串（list item），格式为 Clash classical：

```
TYPE,VALUE,ACTION[,no-resolve]
```

`MATCH,ACTION` 作为兜底规则必须存在（由编译阶段保证）。

---

## 4. 目标：Surge（INI-like）

Surge/Shadowrocket 采用逗号分隔的行语法，渲染时必须额外关注“名称可表示性”。

### 4.1 顶部 `#!MANAGED-CONFIG`（Surge 必须）

当 `mode=config&target=surge`：
- 输出的第一个非空行必须是 `#!MANAGED-CONFIG <CURRENT_CONVERT_URL> ...`。
- `<CURRENT_CONVERT_URL>` 的生成规则见《输出稳定性与规范化规范》，并受 profile 的 `public_base_url` 影响（见《Profile YAML 规范》）。

该行不属于任何锚点注入块，属于 Surge 输出的额外前置内容（见《模板锚点与注入规范》）。

### 4.2 proxiesBlock（写入 `[Proxy]` 段）

#### 4.2.1 内置项

渲染器必须在 `[Proxy]` 段内提供至少两个内置项（用于规则与组引用）：

```
DIRECT = direct
REJECT = reject
```

#### 4.2.2 SS 节点行

每个 SS 节点输出为一行：

```
<NAME> = ss, <SERVER>, <PORT>, encrypt-method=<CIPHER>, password=<PASSWORD>[, <EXTRA_KV>...]
```

可选追加字段（出现时必须是 `, key=value` 或 `, key=true|false` 形式）：
- `udp-relay=true|false`
- `tfo=true|false`

SS plugin（Surge）：
- v1 仅支持 `simple-obfs` / `obfs-local`。
- 当节点携带该 plugin 时，将其 options 以 Surge 参数形式追加（典型为 `obfs=...`、`obfs-host=...`）。
- 其它 plugin：必须报错（`UNSUPPORTED_PLUGIN`）。

#### 4.2.3 名称可表示性（Surge）

由于 Surge 语法使用 `NAME = ...` 与逗号分隔成员列表，v1 约束：
- 策略组名（来自 profile）与规则 action（组名）不得包含 `,` 或 `=` 或控制字符；否则必须报错（用户可改 profile）。
- 节点名（来自订阅）若包含 `,`，必须用双引号包裹：`"a,b" = ss, ...`，并在所有引用处使用同样的带引号名称。
- 节点名若包含 `"` 则必须报错（无可靠转义规则，避免生成歧义配置）。

（节点名中的 `=` 已在规范化阶段替换为 `-`，见《输出稳定性与规范化规范》。）

### 4.3 groupsBlock（写入 `[Proxy Group]` 段）

#### 4.3.1 select

```
<GROUP> = select, <MEMBER_1>, <MEMBER_2>, ...
```

#### 4.3.2 url-test

```
<GROUP> = url-test, <MEMBER_1>, <MEMBER_2>, ..., url=<URL>, interval=<SEC>[, tolerance=<MS>]
```

成员允许：
- 节点名（按 Surge 名称规则可能带引号）
- 其他组名
- `DIRECT` / `REJECT`

### 4.4 rulesBlock（写入 `[Rule]` 段）

规则行语法：

```
TYPE,VALUE,ACTION[,no-resolve]
```

规则类型映射（从 IR 到 Surge）：
- `MATCH` -> `FINAL`
- 其它类型原样输出（`DOMAIN/DOMAIN-SUFFIX/DOMAIN-KEYWORD/IP-CIDR/GEOIP`）

---

## 5. 目标：Shadowrocket（INI-like）

v1 规定 Shadowrocket 的渲染语法与 Surge 相同（使用 `[Proxy]` / `[Proxy Group]` / `[Rule]` 三段）。

差异点：
- 不要求输出 `#!MANAGED-CONFIG`（Shadowrocket 订阅更新机制与 Surge 不同）。
- 其余 proxies/groups/rules 的行语法与名称约束与 Surge 相同。
