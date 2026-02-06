# HTTP API 规范（v1）

本服务提供“订阅转换”能力，严格模式（唯一模式）：任何错误都必须返回 HTTP 错误与结构化信息，便于用户修改远程文件。

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

---

## 2. GET 接口（用于 UI 生成 URL / 便于订阅）

### 2.1 `GET /sub`

查询参数：
- `mode`（必填）：`config` | `list`
- `target`（`mode=config` 必填）：`clash` | `shadowrocket` | `surge`
- `sub`（必填，可重复）：订阅 URL（允许多次传入，表示合并）
- `profile`（`mode=config` 必填）：profile YAML 的 URL
- `encode`（`mode=list` 可选）：`base64` | `raw`（默认 `base64`）

行为：
- `mode=list`：只拉取/解析订阅，输出 ss:// 节点列表（`encode` 控制是否 base64）。
- `mode=config`：拉取/解析订阅 + 拉取/解析 profile + 拉取模板 + 拉取 ruleset，编译后输出目标配置文件。
  - 若 `target=surge`，服务端必须确保输出的第一个非空行是当前请求对应的 `#!MANAGED-CONFIG <URL> ...`（用于 Surge 定时更新）。

示例：

```
/sub?mode=list&sub=https%3A%2F%2Fexample.com%2Fss.txt
/sub?mode=config&target=shadowrocket&sub=https%3A%2F%2Fexample.com%2Fss.txt&profile=https%3A%2F%2Fexample.com%2Frules.yaml
/sub?mode=config&target=surge&sub=https%3A%2F%2Fexample.com%2Fss.txt&profile=https%3A%2F%2Fexample.com%2Frules.yaml
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
  "encode": "base64"
}
```

字段说明：
- `mode`：同 GET
- `target`：同 GET（`mode=config` 必填）
- `subs`：同 GET 的多 `sub` 合并
- `profile`：同 GET
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
    "stage": "parse_ruleset",
    "url": "https://example.com/Proxy.list",
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
- `fetch_ruleset` / `parse_ruleset`
- `compile`
- `render`

---

## 5. 状态码与错误码建议

状态码建议（最小集合）：
- `400`：请求参数不合法（缺字段/枚举值不支持/JSON 非法）
- `422`：远程内容或语义校验失败（profile/模板/ruleset/订阅内容解析失败、引用不成立等）
- `502`：拉取远程资源失败（非超时，如连接失败、DNS 失败、上游返回 5xx 等）
- `504`：拉取远程资源超时
- `500`：服务端内部错误（bug）

错误码建议（可随实现扩展，但避免爆炸式增长）：
- `INVALID_ARGUMENT`
- `FETCH_FAILED`
- `FETCH_TIMEOUT`
- `TOO_LARGE`
- `SUB_PARSE_ERROR`
- `PROFILE_PARSE_ERROR`
- `PROFILE_VALIDATE_ERROR`
- `TEMPLATE_ANCHOR_MISSING`
- `TEMPLATE_ANCHOR_DUP`
- `TEMPLATE_SECTION_ERROR`（Shadowrocket 锚点不在对应 section）
- `RULESET_PARSE_ERROR`
- `RULE_PARSE_ERROR`
- `GROUP_PARSE_ERROR`
- `GROUP_UNSUPPORTED_TYPE`
- `REFERENCE_NOT_FOUND`
- `UNSUPPORTED_TARGET`

---

## 6. 兼容与稳定性要求

- 对同一组输入（订阅/profile/template/ruleset 内容一致），输出必须字节级稳定。
- 允许服务端对远程资源做缓存与并发去重；客户端不得依赖缓存行为（缓存属于实现细节）。
