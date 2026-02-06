# Clash classical 规则规范（v1）

本文档定义本项目支持的 **Clash classical 规则行** 子集，以及 ruleset 文件与 profile inline rule 的解析约束。

核心目标：把“规则语法”锁死在一个地方（解析器），其余模块只处理结构化 `Rule` 列表。

严格模式（唯一模式）：任何规则语法不合法、字段数量不匹配、值无法解析等，必须直接返回 HTTP 错误（错误结构见 `SPEC_HTTP_API.md`）。

---

## 1. 输入位置（哪里会用到这套规则）

v1 有两种规则来源，统一使用本规范解析：

1) **ruleset 文件内容**（来自 profile 的 `ruleset: ["ACTION,URL"]`）
- 文件按行解析为规则。
- 允许规则行 **缺省 ACTION**（由 profile 的 `ruleset` 指令提供默认 ACTION）。

2) **profile inline rule**（来自 profile 的 `rule: ["..."]`）
- 每一条都必须是“完整规则行”（必须显式包含 ACTION）。
- 最终规则必须包含兜底 `MATCH,<ACTION>`（该要求由 `SPEC_PROFILE_YAML.md` 约束）。

---

## 2. 通用行处理

对输入文本按 `\n` 分行（兼容 CRLF），对每一行：
- 去除行首尾空白字符（空格/Tab/CR）。
- 空行：忽略。
- 注释：若去空白后以 `#` 开头，忽略。
- 其它行：必须匹配“支持的规则语法”，否则报错。

---

## 3. 词法与字段分割

规则行以英文逗号 `,` 分割字段：
- 分割后每个字段都要 `trim`（去首尾空白）。
- v1 不支持引号/转义；因此字段值中不得包含 `,`。

字段名约定：
- `TYPE`：规则类型
- `VALUE`：规则值（domain/suffix/keyword/cidr/cc）
- `ACTION`：动作（`DIRECT`/`REJECT`/策略组名）
- `OPT`：可选项（v1 仅支持 `no-resolve`）

---

## 4. 支持的规则类型（v1 子集）

v1 仅支持以下规则类型（`TYPE` 大小写不敏感，但输出建议使用大写）：

### 4.1 `DOMAIN`

完整规则行：

```
DOMAIN,<domain>,<action>
```

ruleset 可缺省 action：

```
DOMAIN,<domain>
```

### 4.2 `DOMAIN-SUFFIX`

完整规则行：

```
DOMAIN-SUFFIX,<suffix>,<action>
```

ruleset 可缺省 action：

```
DOMAIN-SUFFIX,<suffix>
```

### 4.3 `DOMAIN-KEYWORD`

完整规则行：

```
DOMAIN-KEYWORD,<keyword>,<action>
```

ruleset 可缺省 action：

```
DOMAIN-KEYWORD,<keyword>
```

### 4.4 `IP-CIDR`

完整规则行：

```
IP-CIDR,<cidr>,<action>[,no-resolve]
```

ruleset 可缺省 action（但不允许“只有 no-resolve 没有 action”的歧义写法）：

```
IP-CIDR,<cidr>
```

说明：
- `<cidr>` 必须是合法 CIDR（IPv4）。
- `no-resolve` 仅允许出现在第 4 个字段，且仅允许用于 `IP-CIDR`。

### 4.5 `GEOIP`

完整规则行：

```
GEOIP,<cc>,<action>
```

ruleset 可缺省 action：

```
GEOIP,<cc>
```

说明：
- `<cc>` 为国家/地区代码（建议按 ISO 3166-1 alpha-2 的大写写法；v1 不强制校验集合，但必须非空且不得包含空白/逗号）。

### 4.6 `MATCH`（兜底）

完整规则行：

```
MATCH,<action>
```

约束（v1 强制）：
- `MATCH` 不允许缺省 action。
- `MATCH` 语义是兜底，最终规则中必须存在且应当在最后（顺序由 `SPEC_DETERMINISM.md` 约束）。
- ruleset 文件中出现 `MATCH` 必须报错（避免在 ruleset 展开阶段提前“吞掉”后续规则，造成难以诊断的行为）。

---

## 5. ACTION 的约束

`ACTION` 字段必须是以下之一：
- 内置：`DIRECT`、`REJECT`（大小写不敏感；建议规范化为大写）
- 或 profile 定义的策略组名（大小写敏感）

引用是否存在由编译阶段校验（见 `SPEC_PROFILE_YAML.md` 与 `SPEC_DETERMINISM.md` 的命名空间约束）。

---

## 6. 必须报错的情况（最小集合）

- 规则行字段数量不匹配（例如 `DOMAIN,a` 出现在 inline rule；或 `MATCH,a,b`）
- 不支持的 `TYPE`（v1 不做“兼容猜测”）
- `no-resolve` 出现在非 `IP-CIDR` 规则，或出现在错误的位置
- `IP-CIDR` 的 `<cidr>` 不是合法 CIDR
- `MATCH` 出现在 ruleset 文件中
- ruleset 缺省 action 场景下出现歧义写法：`IP-CIDR,<cidr>,no-resolve`（缺 action）

错误响应结构见 `SPEC_HTTP_API.md`；必须尽可能包含 `url/line/snippet/stage=parse_ruleset|compile`。

