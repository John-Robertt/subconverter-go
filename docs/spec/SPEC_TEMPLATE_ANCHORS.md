# 模板锚点与注入规范（v1）

本项目的模板是**纯文本**（YAML/INI/CONF 都按文本处理），服务端不执行模板语言，只做固定锚点替换（注入）。

模板的职责：提供目标客户端配置的静态骨架（全局字段、注释、段落结构等）。  
服务端的职责：生成三段文本（节点/策略组/规则）并注入模板。

---

## 1. 锚点（Anchors）

v1 约定支持以下锚点（`mode=config` 模板必需的最小集合见下）：

- `#@PROXIES@#`：节点列表注入点
- `#@GROUPS@#`：策略组列表注入点
- `#@RULESETS@#`：远程 ruleset 列表注入点（仅 Quantumult X 使用，见第 6 节）
- `#@RULES@#`：规则列表注入点

锚点约束（v1 强制）：
- `#@PROXIES@#` / `#@GROUPS@#` / `#@RULES@#`：必须出现且仅出现一次；缺失或重复都必须报错。
- `#@RULESETS@#`：当 `target=quanx` 时必须出现且仅出现一次；其它 target 出现该锚点视为模板错误。
- 所有锚点必须 **独占一行**（该行除空白外不得包含其它字符）；否则必须报错。

---

## 2. 注入算法（文本层）

输入：
- `templateText`：模板原文（保留原始换行风格）
- 生成文本块（均为“未缩进”文本，以 `\n` 分行）：
  - `proxiesBlock`
  - `groupsBlock`
  - `rulesBlock`
  - `rulesetsBlock`（可选：仅 `target=quanx` 注入到 `[filter_remote]`）

算法要求：
1) 在模板中定位锚点行，记录该行的**前导空白缩进**（indent = leading spaces/tabs）。
2) 用生成块替换整行锚点（包含其换行符）。
3) 替换时，对生成块的每一行添加相同的 indent 前缀（保持 YAML/段落缩进正确）。
4) 输出整体换行风格应与模板一致（若模板主要使用 CRLF，则输出也使用 CRLF；否则使用 LF）。

注意：
- 生成块允许为空，但是否允许为空由编译阶段语义决定（例如 rules 为空应报错；某些 target 可能允许 groups 为空，但 v1 建议也报错）。

---

## 3. Clash（YAML）模板约定（推荐写法）

Clash 模板通常包含三处注入点：

```yaml
proxies:
  #@PROXIES@#
proxy-groups:
  #@GROUPS@#
rules:
  #@RULES@#
```

约定说明：
- 锚点行建议写成注释（`#...`），这样模板本身依然是可读的 YAML。
- 锚点行的缩进决定注入块的缩进。示例里锚点缩进为两个空格，因此注入内容也会以两个空格开头。
- 注入块内部每行应是合法 YAML 列表项（例如 `- {name: ..., type: ss, ...}` 或 `- DOMAIN-SUFFIX,example.com,PROXY`）。

常见错误（必须报错）：
- 锚点不在对应 key 下方的列表区域（会导致输出 YAML 语义错误）。v1 不做 YAML 结构理解，但必须做最小文本校验：锚点行缩进必须大于 0（减少明显误用），其余由用户模板自行保证。

---

## 4. Shadowrocket 模板约定（强制段落位置）

Shadowrocket 配置要求节点/组/规则放在对应 section 内（语法上类似 Surge）。

v1 强制要求锚点分别出现在以下 section 中（忽略大小写）：
- `#@PROXIES@#` 必须位于 `[Proxy]` 段内
- `#@GROUPS@#` 必须位于 `[Proxy Group]` 段内
- `#@RULES@#` 必须位于 `[Rule]` 段内

最小模板示例：

```ini
[General]
loglevel = notify

[Proxy]
#@PROXIES@#

[Proxy Group]
#@GROUPS@#

[Rule]
#@RULES@#
```

v1 的实现应当对 section 位置进行文本级校验（否则用户很难排错）：
- 若锚点不在要求的 section 内 → 必须报错（指出锚点与 section）。

---

## 5. Surge 模板约定（强制段落位置 + 可选 MANAGED-CONFIG）

Surge 配置语法与 Shadowrocket 类似，节点/组/规则也必须位于对应 section 内。

v1 强制要求锚点分别出现在以下 section 中（忽略大小写）：
- `#@PROXIES@#` 必须位于 `[Proxy]` 段内
- `#@GROUPS@#` 必须位于 `[Proxy Group]` 段内
- `#@RULES@#` 必须位于 `[Rule]` 段内

### 5.1 `#!MANAGED-CONFIG`（由编译器动态生成）

Surge 支持在配置文件顶部添加一行 managed config 指令，用于客户端定时更新配置。

当 `target=surge` 且 `mode=config` 时，转换服务必须确保输出的第一个非空行是：

```
#!MANAGED-CONFIG <CURRENT_CONVERT_URL> interval=86400
```

说明：
其中 `<CURRENT_CONVERT_URL>` 是“当前请求对应的订阅转换链接”，通常等价于本次请求的 `GET /sub?...` URL（需要包含 `mode=config&target=surge` 以及订阅/profile 等参数）。
`<CURRENT_CONVERT_URL>` 的稳定生成规则见《输出稳定性与规范化规范》；当 profile 提供 `public_base_url` 时必须使用它作为 base URL。

约束（v1）：
- 若模板的第一个非空行已经是 `#!MANAGED-CONFIG ...`，服务端必须**重写其中的 URL** 为 `<CURRENT_CONVERT_URL>`，并保留其余参数（如 `interval=...`）。
- 若模板未提供 `#!MANAGED-CONFIG ...` 行，服务端必须在输出顶部自动插入一行（使用 v1 默认参数）。
- 模板若包含多条 `#!MANAGED-CONFIG`，或该行未位于第一个非空行，必须报错（避免歧义）。

---

## 6. Quantumult X 模板约定（强制段落位置）

Quantumult X（QuanX）配置中，节点/策略组/规则分别位于不同 section。

v1 强制要求锚点分别出现在以下 section 中（忽略大小写）：
- `#@PROXIES@#` 必须位于 `[server_local]` 段内
- `#@GROUPS@#` 必须位于 `[policy]` 段内
- `#@RULESETS@#` 必须位于 `[filter_remote]` 段内
- `#@RULES@#` 必须位于 `[filter_local]` 段内

最小模板示例：

```ini
[policy]
#@GROUPS@#

[server_local]
#@PROXIES@#

[filter_remote]
#@RULESETS@#

[filter_local]
#@RULES@#
```

---

## 7. 错误要求

模板相关必须报错的情况：
- 必需锚点（`#@PROXIES@#/#@GROUPS@#/#@RULES@#`）缺失/重复/不独占一行
- `target=quanx` 时必需锚点 `#@RULESETS@#` 缺失/重复/不独占一行
- Shadowrocket 模板中锚点未出现在要求的 section 内
- Surge 模板中锚点未出现在要求的 section 内
- Quantumult X 模板中锚点未出现在要求的 section 内
- Surge 模板 `#!MANAGED-CONFIG` 行存在歧义（多条、或未位于第一个非空行）
- 模板拉取失败或内容为空（空模板视为错误）

错误响应 JSON 结构在《HTTP API 规范》中定义。
