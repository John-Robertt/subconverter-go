# subconverter-go

一个订阅转换 HTTP 服务：输入 **SS 订阅**（`ss://` 节点列表或其 base64 形式），按远程 `profile.yaml` 的策略组/规则描述进行编译，输出各客户端可直接导入的配置文件：

- Clash（mihomo）
- Surge
- Shadowrocket（iOS）
- Quantumult X（QuanX）

也支持输出“纯节点列表”（用于二次分发/导入）。

## 这项目解决什么问题

你只需要维护两样东西：

1) **订阅**：节点列表（SS）  
2) **profile**：策略组/规则/模板 URL（远程托管）

服务端负责把它们“拼装”为目标客户端配置；不执行任何远程内容，只做解析与组装。

## 快速开始（3 步）

1) 编译：

```bash
go build -o subconverter-go ./cmd/subconverter-go
```

2) 启动：

```bash
./subconverter-go -listen 127.0.0.1:25500
```

可选：调整超时边界（示例）：

```bash
./subconverter-go \
  -error-log-dir ./logs \
  -convert-timeout 60s \
  -fetch-timeout 15s \
  -shutdown-timeout 10s
```

3) 访问：

```bash
curl -fsS http://127.0.0.1:25500/healthz
```

可选：查看内置指标（Prometheus text format，最小集合）：

```bash
curl -fsS http://127.0.0.1:25500/metrics
```

可选：打开内置 UI（生成订阅转换链接）：

```text
http://127.0.0.1:25500/
```

可选：下载错误快照日志 ZIP：

```bash
curl -fsS -o subconverter-errors.zip http://127.0.0.1:25500/logs/errors.zip
```

说明：
- `-error-log-dir` 未指定时，默认当前工作目录。
- 启动前会检查日志目录是否可读可写；若不可用，服务会直接退出并提示容器挂载可写卷或调整 `WORKDIR` / `-error-log-dir`。
- 运行期间若日志目录失效，或错误快照写入/导出出现真实 I/O 故障，`/healthz` 会返回 `503`，便于容器健康检查接管重启。
- 正常情况下，`/healthz` 不会创建空的 `errors-YYYY-MM-DD.jsonl`；只有真实转换失败时才会生成错误日志文件。

## Docker 运行（推荐部署）

镜像发布在 GitHub Container Registry（GHCR）：

- `ghcr.io/john-robertt/subconverter-go:latest`
- `ghcr.io/john-robertt/subconverter-go:vX.Y.Z`

直接运行：

```bash
docker run --rm -p 25500:25500 ghcr.io/john-robertt/subconverter-go:latest
```

容器内默认监听 `0.0.0.0:25500`（可用 `-listen` 参数覆盖）。

若需要保留错误快照日志，建议挂载一个可写目录并传入 `-error-log-dir`：

```bash
docker run --rm \
  -p 25500:25500 \
  -v "$PWD/logs:/logs" \
  ghcr.io/john-robertt/subconverter-go:latest \
  -listen 0.0.0.0:25500 \
  -error-log-dir /logs
```

### Docker Compose：健康检查（scratch 镜像）

镜像使用 `scratch` 作为运行时基础镜像（体积小、攻击面小），因此容器内**没有** `/bin/sh`、`curl` 等工具。

如果你希望启用 `healthcheck`，请使用内置的 `-healthcheck` 模式：

```yaml
healthcheck:
  test: [ "CMD", "/subconverter-go", "-healthcheck" ]
  interval: 10s
  timeout: 3s
  retries: 20
```

若你覆盖了服务端口，请在健康检查里同步传入相同的 `-listen`（仅用于推导要检查的 `127.0.0.1:<port>/healthz`）：

```yaml
healthcheck:
  test: [ "CMD", "/subconverter-go", "-healthcheck", "-listen", "0.0.0.0:25500" ]
```

## 预编译二进制（GitHub Releases）

每次 push tag（`v*`）会自动生成 GitHub Release，包含：

- Linux / macOS（darwin）/ Windows 的 `amd64`、`arm64` 二进制包
- 每个产物的 SHA256 校验文件（`*_SHA256SUMS.txt`）

## 使用方式

### 1) 输出配置文件（mode=config）

GET 接口（便于“生成一个 URL 直接订阅”）：

```bash
curl -G 'http://127.0.0.1:25500/sub' \
  --data-urlencode 'mode=config' \
  --data-urlencode 'target=clash' \
  --data-urlencode 'sub=https://example.com/ss.txt' \
  --data-urlencode 'profile=https://example.com/profile.yaml'
```

参数说明：
- `target`: `clash|surge|shadowrocket|quanx`
- `sub`: 订阅 URL，可重复传多个（按出现顺序合并）
- `profile`: profile YAML 的 URL
- `fileName`（可选）：自定义下载文件名（服务端会按 target 自动补扩展名；Surge 的 `#!MANAGED-CONFIG` URL 也会携带该参数）

### 2) 输出纯节点列表（mode=list）

raw（明文，每行一个 `ss://...`，**末尾带换行**）：

```bash
curl -G 'http://127.0.0.1:25500/sub' \
  --data-urlencode 'mode=list' \
  --data-urlencode 'encode=raw' \
  --data-urlencode 'sub=https://example.com/ss.txt'
```

base64（默认，内容为 raw 文本的标准 base64）：

```bash
curl -G 'http://127.0.0.1:25500/sub' \
  --data-urlencode 'mode=list' \
  --data-urlencode 'sub=https://example.com/ss.txt'
```

### 3) POST 接口（长参数/批量）

```bash
curl -fsS 'http://127.0.0.1:25500/api/convert' \
  -H 'Content-Type: application/json' \
  -d '{
    "mode": "config",
    "target": "surge",
    "subs": ["https://example.com/ss.txt"],
    "profile": "https://example.com/profile.yaml",
    "fileName": "my_surge"
  }'
```

### 4) 下载错误日志 ZIP

```bash
curl -fsS -o subconverter-errors.zip \
  'http://127.0.0.1:25500/logs/errors.zip'
```

返回内容：
- `application/zip`
- ZIP 内只包含按天切分的 `errors-YYYY-MM-DD.jsonl`
- 若当前还没有真实失败日志，接口返回 `404`

## profile.yaml（最小可用示例）

profile 是一个 YAML 文件，但写法尽量贴近 `Rules.ini` 的“指令列表”。

```yaml
version: 1

template:
  clash: "https://example.com/templates/clash.yaml"
  surge: "https://example.com/templates/surge.conf"
  shadowrocket: "https://example.com/templates/shadowrocket.conf"
  quanx: "https://example.com/templates/quanx.conf"

# Surge 的 #!MANAGED-CONFIG 会使用这个 base URL（建议填你的公网域名 + /sub）
public_base_url: "https://sub-api.example.com/sub"

custom_proxy_group:
  - "PROXY`select`[]AUTO[]@all[]DIRECT"
  - "AUTO`url-test`(HK|SG|US)`http://www.gstatic.com/generate_204`300`50"
  # 也支持 select 的正则筛选写法（从节点名里筛选）
  - "🇭🇰 Hong Kong`select`(港|HK|Hong Kong)"

ruleset:
  - "DIRECT,https://example.com/rulesets/LAN.list"
  - "REJECT,https://example.com/rulesets/BanAD.list"
  - "PROXY,https://example.com/rulesets/Proxy.list"

rule:
  - "MATCH,PROXY"
```

重要约束（v1）：
- `rule` 必须包含兜底 `MATCH,<ACTION>`，否则直接报错（避免生成不可控配置）
- `ruleset` 在 v1 **不由服务端拉取/校验内容**：只负责“引用 + 绑定 ACTION + 顺序”；确保你的客户端能访问这些 ruleset URL

完整规范见：`docs/spec/SPEC_PROFILE_YAML.md`

## 模板（anchors）怎么写

模板是纯文本；服务端只做锚点替换（注入），不执行模板语言。

锚点必须**独占一行**，且必须出现在正确位置（否则会返回模板错误）。

### Clash 模板（必须包含 RULE_PROVIDERS）

```yaml
proxies:
  #@PROXIES@#
proxy-groups:
  #@GROUPS@#
rule-providers:
  #@RULE_PROVIDERS@#
rules:
  #@RULES@#
```

其它 target 的锚点位置规则见：`docs/spec/SPEC_TEMPLATE_ANCHORS.md`

## Clash rule-providers（mihomo）说明

为了避免把大型 ruleset 展开成几十万行，Clash 输出采用：
- `rule-providers:`：每个 `ruleset` URL 生成一个 provider
- `rules:`：按顺序输出 `RULE-SET,<providerName>,<ACTION>`，再追加 profile 的 inline rules

`providerName` 由 URL path 的文件名确定性生成（重名会追加 `-2/-3/...`）。

## 错误如何排查

任何解析/校验失败都会返回结构化 JSON（便于你修改远程文件），例如：
- `code`：错误码（如 `PROFILE_PARSE_ERROR`、`TEMPLATE_ANCHOR_MISSING`）
- `stage`：出错阶段（如 `parse_profile`、`validate_template`）
- `url/line/snippet/hint`：定位信息（如果适用）

同时服务端会输出最小访问日志（不包含 query string，避免泄露 sub/profile 里的 token）。
对于转换失败，服务端还会把脱敏后的错误快照追加到按天日志文件中，并可通过 `GET /logs/errors.zip` 打包下载。

错误快照与健康检查的关系：
- 只有真实转换失败时，才会追加 `errors-YYYY-MM-DD.jsonl`
- `/healthz` 在健康状态下只做无副作用检查，不会创建空日志文件
- 若某次错误快照写入失败，服务仍会把原始业务错误返回给客户端，但随后 `/healthz` 会进入 `503`，让容器或外部探针发现“错误日志持久化能力已失效”
- 当日志目录恢复可读写后，`/healthz` 会自动恢复为 `200`

见：`docs/spec/SPEC_HTTP_API.md`

## 安全提示（很重要）

本服务会主动拉取这些 URL（仅 `http/https`）：
- `sub=...`（订阅）
- `profile=...`（profile YAML）
- profile 里的 `template.*`（模板）

并且 v1 **允许内网访问**（为了支持“订阅在内网”的场景）。这意味着：如果你把服务暴露到不可信网络，它天然具备 SSRF 风险。

建议的最小安全姿势：
- 默认只监听 `127.0.0.1`，需要对外提供时放到反代后并加鉴权（token/白名单/隔离网络）

## 文档与材料

- 文档入口：`docs/README.md`
- 测试材料：`docs/materials/`
- 真实订阅端到端生成：`scripts/gen_real.sh`

## 自测（建议）

```bash
go test ./...
```

可选：跑 fuzz（示例，30 秒）：

```bash
go test ./internal/sub/ss -fuzz=FuzzParseSubscriptionText -fuzztime=30s
```

可选：放一个真实订阅到仓库根目录 `SS.txt`（已在 `.gitignore` 中忽略），然后：

```bash
bash scripts/gen_real.sh
```

生成的配置会写到 `out/real/`。
