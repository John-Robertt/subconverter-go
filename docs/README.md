# 文档入口

## 0. 文档分层（不要混写）

本项目把文档分成三层，避免“规范/设计/计划”揉成一团：

- **规范（SPEC_*.md）**：定义外部可观察行为（what）。必须/禁止/允许写在这里；它是测试与验收基准。
- **设计（ARCHITECTURE/DESIGN_*.md）**：解释为什么这样定、最小拆分怎么落地（why/how）。不要在这里偷偷新增 MUST 行为。
- **开发与验证（DEV_*.md / DECISIONS.md）**：定义分阶段交付与每阶段验收（when/verify），以及关键决策的证据链（why）。

改行为的规矩：
1) 先改 SPEC（或补 DECISIONS 解释取舍）  
2) 再改实现  
3) 用测试证明变更是刻意且可回归的（见 `dev/DEV_GUIDE.md`）  

## 1. 开发与验证（DEV）

- 开发方法与分阶段验收（v1）：`dev/DEV_GUIDE.md`
- 决策记录（v1）：`dev/DECISIONS.md`

## 2. 设计（DESIGN）

- 架构设计（v1）：`design/ARCHITECTURE.md`
- 代码目录与模块职责（v1 设计建议）：`design/DESIGN_CODE_LAYOUT.md`

## 3. 规范（SPEC）

- HTTP API 规范（v1）：`spec/SPEC_HTTP_API.md`
- 远程拉取（Fetch）规范（v1）：`spec/SPEC_FETCH.md`
- SS 订阅解析规范（v1）：`spec/SPEC_SUBSCRIPTION_SS.md`
- Profile YAML 规范（v1）：`spec/SPEC_PROFILE_YAML.md`
- Clash classical 规则规范（v1）：`spec/SPEC_RULES_CLASH_CLASSICAL.md`
- 输出稳定性与规范化规范（v1）：`spec/SPEC_DETERMINISM.md`
- 渲染规范（v1，Clash/Surge/Shadowrocket）：`spec/SPEC_RENDER_TARGETS.md`
- 模板锚点与注入规范（v1）：`spec/SPEC_TEMPLATE_ANCHORS.md`
- 安全与内网访问规范（v1）：`spec/SPEC_SECURITY.md`

## 4. 测试材料

- 测试材料（v1）：`materials/README.md`
