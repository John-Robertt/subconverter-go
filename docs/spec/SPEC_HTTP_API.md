# HTTP API 规范（v1）

本服务提供“订阅转换”能力：任何错误都必须返回 HTTP 错误与结构化信息，便于用户修改远程文件。

v1 约定：
- profile inline rule（`profile.rule`）：语法不合法 / 不支持的规则类型 → 直接报错。
- ruleset（`profile.ruleset`）：v1 默认**不拉取、不解析**远程 ruleset 文件内容，仅做“引用/绑定/排序”：
  - Clash（mihomo）：输出 `rule-providers` + `RULE-SET,<PROVIDER_NAME>,<ACTION>`
  - Surge/Shadowrocket：输出 `RULE-SET,<URL>,<ACTION>`
  - Quantumult X：输出到 `[filter_remote]` 的远程引用行

因此：
- ruleset 文件内部的语法错误不会在服务端提前暴露（由客户端在拉取/更新时自行报错）。
- 服务端仍必须校验 `ruleset` 指令本身的语法，并校验 `ACTION` 引用必须存在（组名/DIRECT/REJECT）。
- `proxy_chain` 当前仅对 `target=clash|surge` 生效；profile 使用该特性而目标不支持时，服务端必须返回业务错误。

---

## 1. 基本约定

- 成功响应：
  - `200 OK`
  - `Content-Type: text/plain; charset=utf-8`
  - body 为配置文件或节点列表的**纯文本**
- 失败响应：
  - `4xx/5xx`
  - `Content-Type: application/json; charset=utf-8`
  - body 为结构化错误（见第 4 节）

辅助接口：
- `GET /healthz`
  - 健康时返回 `200 OK` + `ok\n`
  - 当错误日志目录失效，或错误快照持久化能力已降级时，返回 `503 Service Unavailable`
  - 健康状态下不得创建业务错误日志文件；即 `/healthz` 不能因为探测本身生成空的 `errors-YYYY-MM-DD.jsonl`
- `GET /logs/errors.zip`
  - 成功时返回 `200 OK` + `application/zip`
  - ZIP 内只包含真实产生的 `errors-YYYY-MM-DD.jsonl`
  - 若当前没有任何错误日志文件，返回 `404`

---

## 2. GET 接口（用于 UI 生成 URL / 便于订阅）

### 2.1 `GET /sub`

查询参数：
- `mode`（必填）：`config` | `list`
- `target`（`mode=config` 必填）：`clash` | `shadowrocket` | `surge` | `quanx`
- `sub`（必填，可重复）：订阅 URL（允许多次传入，表示合并）
- `profile`（`mode=config` 必填）：profile YAML 的 URL
- `encode`（`mode=list` 可选）：`base64` | `raw`（默认 `base64`）
- `fileName`（可选）：生成文件名（不含路径；通常不需要带扩展名）。缺省时服务端使用默认文件名：
  - `mode=list`：`ss.txt`
  - `mode=config`：按 target 选择扩展名（例如 `clash.yaml`、`surge.conf`、`shadowrocket.conf`、`quanx.conf`）

行为：
- `mode=list`：只拉取/解析订阅，输出 ss:// 节点列表（`encode` 控制是否 base64）。
- `mode=config`：拉取/解析订阅 + 拉取/解析 profile + 拉取模板，编译后输出目标配置文件（v1 默认不拉取、不展开 ruleset 内容）。
  - 若 `target=surge`，服务端必须确保输出的第一个非空行是当前请求对应的 `#!MANAGED-CONFIG <URL> ...`（用于 Surge 定时更新）。
    - `<URL>` 的 base URL 若 profile 提供 `public_base_url`，必须使用该字段（见《Profile YAML 规范》）。
  - 服务端应设置 `Content-Disposition`（attachment）。若提供 `fileName`，则使用它作为文件名（并按 target/mode 自动补扩展名）；若缺省则使用默认文件名。
  - 若提供 `fileName` 且 `target=surge`，服务端必须在 Surge managed-config URL 中携带该参数，以便后续更新保持一致。

示例：

```
/sub?mode=list&sub=https%3A%2F%2Fexample.com%2Fss.txt
/sub?mode=config&target=shadowrocket&sub=https%3A%2F%2Fexample.com%2Fss.txt&profile=https%3A%2F%2Fexample.com%2Frules.yaml
/sub?mode=config&target=surge&sub=https%3A%2F%2Fexample.com%2Fss.txt&profile=https%3A%2F%2Fexample.com%2Frules.yaml
/sub?mode=config&target=clash&sub=https%3A%2F%2Fexample.com%2Fss.txt&profile=https%3A%2F%2Fexample.com%2Frules.yaml
/sub?mode=config&target=surge&fileName=my_surge&sub=https%3A%2F%2Fexample.com%2Fss.txt&profile=https%3A%2F%2Fexample.com%2Frules.yaml
```

---

## 3. POST 接口（用于长参数/批量）

### 3.1 `POST /api/convert`

请求：
- `Content-Type: application/json`

body：

```json
{
  "mode": "config",
  "target": "shadowrocket",
  "subs": ["https://example.com/ss.txt"],
  "profile": "https://example.com/rules.yaml",
  "fileName": "my_shadowrocket",
  "encode": "base64"
}
```

字段说明：
- `mode`：同 GET
- `target`：同 GET（`mode=config` 必填）
- `subs`：同 GET 的多 `sub` 合并
- `profile`：同 GET
- `fileName`：同 GET
- `encode`：仅 `mode=list` 生效

响应：同第 1 节约定。

备注：
- 若 `mode=config&target=surge`，服务端应生成一个等价的 `GET /sub?...` URL 并写入 `#!MANAGED-CONFIG` 行（Surge 只能通过 URL 拉取更新）。

---

## 4. 错误响应结构（强制）

统一错误结构：

```json
{
  "error": {
    "code": "RULE_PARSE_ERROR",
    "message": "invalid rule line",
    "stage": "parse_profile",
    "url": "https://example.com/profile.yaml",
    "line": 123,
    "snippet": "DOMAIN-SUFFIX,google.com",
    "hint": "expected: TYPE,VALUE[,ACTION][,no-resolve]"
  }
}
```

字段含义：
- `code`：机器可读错误码（见第 5 节建议集合）
- `message`：人类可读错误信息（中文）
- `stage`：出错阶段（枚举，便于定位）
- `url`：相关远程资源 URL（若适用）
- `line`：1-based 行号（若适用）
- `snippet`：出错行片段（若适用，建议截断到 <= 200 字符）
- `hint`：修复提示（可选，但建议提供）

`stage` 建议枚举：
- `validate_request`
- `fetch_sub` / `parse_sub`
- `fetch_profile` / `parse_profile`
- `fetch_template` / `validate_template`
- `compile`
- `render`

---

## 5. 状态码与错误码建议

状态码建议（最小集合）：
- `400`：请求参数不合法（缺字段/枚举值不支持/JSON 非法）
- `422`：远程内容或语义校验失败（profile/模板/订阅内容解析失败、引用不成立、目标不支持 `proxy_chain` 等）
- `502`：拉取远程资源失败（非超时，如连接失败、DNS 失败、上游返回 5xx 等）
- `504`：拉取远程资源超时
- `500`：服务端内部错误（bug）

错误码建议（可随实现扩展，但避免爆炸式增长）：
- `INVALID_ARGUMENT`
- `FETCH_FAILED`
- `FETCH_INVALID_UTF8`
- `FETCH_TIMEOUT`
- `TOO_LARGE`
- `SUB_PARSE_ERROR`
- `SUB_BASE64_DECODE_ERROR`
- `SUB_UNSUPPORTED_SCHEME`
- `PROFILE_PARSE_ERROR`
- `PROFILE_VALIDATE_ERROR`
- `TEMPLATE_ANCHOR_MISSING`
- `TEMPLATE_ANCHOR_DUP`
- `TEMPLATE_SECTION_ERROR`（Shadowrocket 锚点不在对应 section）
- `RULESET_PARSE_ERROR`
- `RULE_PARSE_ERROR`
- `GROUP_PARSE_ERROR`
- `GROUP_UNSUPPORTED_TYPE`
- `GROUP_REFERENCE_CYCLE`
- `CUSTOM_PROXY_VALIDATE_ERROR`
- `CHAIN_PARSE_ERROR`
- `CHAIN_PROXY_NOT_FOUND`
- `CHAIN_GROUP_NOT_FOUND`
- `CHAIN_SELECTOR_EMPTY`
- `UNSUPPORTED_PLUGIN`
- `UNSUPPORTED_RULE_TYPE`
- `REFERENCE_NOT_FOUND`
- `UNSUPPORTED_TARGET`
- `UNSUPPORTED_TARGET_FEATURE`

---

## 6. 兼容与稳定性要求

- 对同一组输入（订阅/profile/template/ruleset 内容一致），输出必须字节级稳定。
- 允许服务端对远程资源做缓存与并发去重；客户端不得依赖缓存行为（缓存属于实现细节）。

---

## 7. 错误快照与健康检查约束

- 服务端在转换失败时，应把脱敏后的错误快照追加到按天切分的 `errors-YYYY-MM-DD.jsonl`
- 服务端在返回业务错误给客户端时，不得因为“写错误快照失败”而改写原始业务状态码
- 若错误快照写入失败，服务端必须把错误日志子系统标记为降级，后续 `GET /healthz` 必须返回 `503`
- `GET /healthz` 在正常状态下只能做无副作用检查；只有在已经降级的前提下，才允许通过隐藏探针文件验证恢复能力
- `GET /logs/errors.zip` 的成功，不代表写入能力已经恢复；写入能力恢复应以健康检查中的恢复探针为准
