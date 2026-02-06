# 测试材料（v1）

本目录提供一套“可通过 HTTP 拉取”的最小测试材料，用于联调：
- profile（YAML）
- 模板（Clash / Shadowrocket / Surge）
- 规则集（Clash classical list）
- 订阅（SS 节点列表：明文与 base64）

## 1) 启动一个静态文件服务器（示例）

从本仓库的 `docs` 目录启动：

```bash
cd docs
python3 -m http.server 8000
```

此时这些 URL 将可访问：
- profile：`http://127.0.0.1:8000/materials/profile.yaml`
- Clash 模板：`http://127.0.0.1:8000/materials/templates/clash.yaml`
- Shadowrocket 模板：`http://127.0.0.1:8000/materials/templates/shadowrocket.conf`
- Surge 模板：`http://127.0.0.1:8000/materials/templates/surge.conf`
- ruleset：`http://127.0.0.1:8000/materials/rulesets/*.list`
- 订阅（明文）：`http://127.0.0.1:8000/materials/subscriptions/ss.txt`
- 订阅（base64）：`http://127.0.0.1:8000/materials/subscriptions/ss.b64`

## 2) 使用方式（示例）

假设转换服务提供 `GET /sub`：

```text
/sub?mode=list&sub=http%3A%2F%2F127.0.0.1%3A8000%2Fmaterials%2Fsubscriptions%2Fss.b64
/sub?mode=config&target=shadowrocket&sub=http%3A%2F%2F127.0.0.1%3A8000%2Fmaterials%2Fsubscriptions%2Fss.b64&profile=http%3A%2F%2F127.0.0.1%3A8000%2Fmaterials%2Fprofile.yaml
/sub?mode=config&target=surge&sub=http%3A%2F%2F127.0.0.1%3A8000%2Fmaterials%2Fsubscriptions%2Fss.b64&profile=http%3A%2F%2F127.0.0.1%3A8000%2Fmaterials%2Fprofile.yaml
```

注意：
- 这套材料只是“最小样例”，方便你快速打通 fetch/parse/compile/render 的流水线。
- 本项目是严格模式：改坏任何一行都应该得到结构化错误（带 URL/行号/片段）。
- `target=surge` 时，转换服务需要确保输出的首个非空行是 `#!MANAGED-CONFIG <URL> ...`（URL 为当前请求对应的订阅转换链接）。模板可不包含该行，服务端会自动插入。
