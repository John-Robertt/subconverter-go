# 远程拉取（Fetch）规范（v1）

本文档定义服务端拉取远程资源（订阅/profile/template/ruleset）的最小行为约束：超时、大小上限、重定向与错误分类。

注意：本项目需要支持访问内网（例如订阅 URL 在内网）。因此 v1 不包含“禁止私网/内网 IP”的限制；订阅/profile/template/ruleset **均允许**使用内网或外网 URL。

这也意味着：如果服务对不可信网络开放，将天然具备 SSRF 能力。请通过“默认仅监听 127.0.0.1 / 对外暴露必须鉴权 token”等运行姿势降低风险（见 `SPEC_SECURITY.md`）。

---

## 1. 资源类型

v1 有四类远程文本资源：
- Subscription：订阅（SS）
- Profile：profile YAML
- Template：目标模板（Clash YAML / Shadowrocket/Surge conf）
- Ruleset：规则集（Clash classical list）

---

## 2. URL 与协议

约束（v1 强制）：
- 仅允许 `http://` 与 `https://` URL。
- 其它 scheme（例如 `file://`、`ftp://`、`data:`）必须报错（`INVALID_ARGUMENT`）。

---

## 3. 超时与大小上限

### 3.1 超时

v1 必须设置请求超时（避免上游卡死拖垮服务）。建议默认值：
- 单次 HTTP 请求总超时：15 秒

（是否可配置由实现决定，但必须存在默认值。）

### 3.2 响应大小上限

v1 必须对响应体做硬性大小上限（`io.LimitReader` 类似机制），建议默认值（可调整）：
- Subscription：<= 5 MiB
- Profile：<= 1 MiB
- Template：<= 2 MiB
- Ruleset：<= 10 MiB（部分广告规则集可能超过 5 MiB）

超过上限必须立刻中止并报错（建议错误码：`TOO_LARGE`；HTTP 状态码建议 422 或 502，按实现选择，但需一致）。

---

## 4. 重定向策略

v1 必须限制重定向，建议：
- 最多跟随 5 次重定向。
- 重定向目标 URL 仍必须是 `http/https`，否则报错。

---

## 5. 文本编码与换行

约束（v1 强制）：
- 拉取到的资源必须能按 UTF-8 解码（订阅/profile/template/ruleset 全是文本）。
- 若出现非法 UTF-8 字节序列必须报错（建议错误码：`FETCH_INVALID_UTF8`）。

换行约定：
- 规则/订阅解析以 `\n` 分行，同时兼容 CRLF。
- 模板输出换行风格必须跟随模板原文（见《模板锚点与注入规范》）。

---

## 6. 错误分类（与 HTTP API 对齐）

Fetch 层错误需要映射到 HTTP API 的状态码与错误结构（见《HTTP API 规范》）。

建议映射：
- DNS/连接失败/上游 5xx：`502` + `FETCH_FAILED`
- 超时：`504` + `FETCH_TIMEOUT`
- 超过大小上限：`422`（或 `502`）+ `TOO_LARGE`
- URL 非法/协议不支持：`400` + `INVALID_ARGUMENT`

错误体必须包含：
- `stage=fetch_sub|fetch_profile|fetch_template|fetch_ruleset`
- `url`

---

## 7. 缓存与并发去重（实现建议）

本规范不强制缓存语义（缓存属于实现细节），但 v1 强烈建议：
- 对相同 URL 的并发拉取进行去重（singleflight），避免缓存击穿/上游被打爆。
- 可使用 ETag/Last-Modified 做条件请求，减少带宽。

无论是否缓存，**错误可定位**要求不变：报错必须带上游 URL 与尽可能多的定位信息。
