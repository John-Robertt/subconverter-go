# 安全与内网访问规范（v1）

本文档描述 v1 的安全边界与默认运行姿势。它不是“把风险抹平”，而是把风险**说清楚**并给出最小可行的防踩雷约束。

v1 的关键前提：**允许访问内网 URL**（订阅/profile/template/ruleset 都可能在内网）。

---

## 1. 威胁模型（你必须接受的事实）

本服务对外暴露一个“可控 URL 拉取 + 解析 + 拼装输出”的能力。

v1 服务端会主动发起网络请求的入口只有三处：
- `sub=...`（订阅）
- `profile=...`（profile YAML）
- profile 中的 `template.*`（模板 URL）

`ruleset` 在 v1 默认**不由服务端拉取/解析其远程文件内容**，仅作为 URL 引用渲染到输出里（见 `SPEC_HTTP_API.md` / `SPEC_RENDER_TARGETS.md`）。  
因此：`ruleset` 不增加服务端的 SSRF 面（但客户端在更新配置时仍可能自行拉取 ruleset URL）。

当允许内网 URL 时，如果该服务对不可信网络开放，它天然具备：
- **SSRF**：攻击者可借你服务去探测/访问内网资源。
- **带宽/资源消耗**：可被利用拉取大文件、慢连接，拖垮服务。

v1 的策略是不做“万能安全网”，而是：
- 在协议与资源层做硬约束（超时、大小上限、协议限制、重定向限制）。
- 给出默认运行姿势，避免用户“无意中把 SSRF 服务开到公网”。

---

## 2. 远程内容：只解析、不执行

v1 对远程输入的强约束：
- subscription/profile/template 一律按**纯文本数据**处理（只解析，不执行）。
- ruleset：v1 默认不拉取、不解析远程文件内容，仅把 URL 作为引用写入输出（见 `SPEC_HTTP_API.md` / `SPEC_RENDER_TARGETS.md`）。
- 不支持任何可执行模板语言，不执行 JS/Lua/脚本。
- 输出内容 = “模板原文 + 生成块的锚点注入”（见 `SPEC_TEMPLATE_ANCHORS.md`），不存在“远程内容可被执行”的路径。

---

## 3. URL 访问策略（v1 必须满足）

硬约束（v1 强制）：
- 仅允许 `http://` 与 `https://`（其余 scheme 直接报错，见 `SPEC_FETCH.md`）。
- 必须有超时、响应体大小上限、重定向次数上限（见 `SPEC_FETCH.md`）。

关于“内网/外网”：
- v1 不禁止私网/内网 IP，也不做 DNS 解析后的网段拦截（因为用户明确需要访问内网）。
- 这意味着 **SSRF 风险由部署姿势承担**（见第 4 节）。

---

## 4. 默认运行姿势（强烈建议）

为了让“默认行为”更安全，建议 v1 的默认配置为：
- 默认仅监听 `127.0.0.1`。
- 若对外暴露（反代或直连），必须增加简单鉴权（例如 token），否则此服务可被滥用为内网探测器。

说明：
- 鉴权方案属于实现与部署策略，不强行写死到 v1 的 HTTP API 规范里；但服务端实现应当留出钩子（例如中间件/handler 前置检查）。

补充（容易被忽略）：
- **仅监听 `127.0.0.1` 不是“绝对安全”**：不可信网页仍可能通过 DNS rebinding / CSRF 等方式触发对本机服务的请求，从而间接利用本服务发起 SSRF。  
  如果你的使用场景包含“会打开不可信网页”，同样建议增加鉴权 token，并在反代层做 Host/Origin 限制（或至少不要放开 CORS）。
- 不要给该服务配置 `Access-Control-Allow-Origin: *` 这类宽松 CORS；这等于在浏览器里给攻击者打开“可读回显”的通道。

---

## 5. `public_base_url`：避免“对外 URL 指向内网”

当输出 `target=surge` 且 `mode=config` 时，需要生成 `#!MANAGED-CONFIG <CURRENT_CONVERT_URL> ...`。

如果服务跑在内网或反代后：
- 直接从当前请求推导出来的 base URL 可能是内网地址（例如 `http://127.0.0.1:25500/sub`），对 Surge 客户端不可达。

因此 v1 允许在 profile 提供 `public_base_url`（见 `SPEC_PROFILE_YAML.md`）：
- 一旦提供，必须用它生成 `<CURRENT_CONVERT_URL>`（见 `SPEC_DETERMINISM.md` 的序列化规则）。
- 该字段本质上是“对外可达的订阅转换入口”，不应包含敏感 query（也不得包含 query/fragment）。

安全提示：
- Surge 的 `#!MANAGED-CONFIG` URL 会包含 `sub/profile` 等 query；这些 URL 往往自带访问 token/密钥，因此**输出配置文件应视为敏感信息**，不要公开。
- 若你选择用 token 做对外鉴权，而客户端只能通过 URL 拉取更新（例如 Surge managed-config），token 很可能最终出现在 URL query；同样视为 secret，避免泄露/分享。
