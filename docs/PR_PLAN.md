# QuickGo PR 实施计划

基于项目评审结论，将优化工作拆为可独立 review / 合并的 PR。  
依赖关系：`PR1 → PR2 → PR3` 建议串行；`PR4` 可与 `PR3` 并行；`PR5` 依赖 `PR3` 文档骨架。

---

## 总览

| PR | 标题 | 阶段 | 预估 | 状态 |
|----|------|------|------|------|
| **PR1** | 仓库卫生：gitignore / 构建产物 / 备份文件 | 工程卫生 | 0.5d | 已完成 |
| **PR2** | Makefile + CI + Go toolchain 对齐 | 工程卫生 | 0.5–1d | 已完成 |
| **PR3** | 根 README 能力地图 + logger 文档修正 | 文档 | 0.5–1d | 已完成 |
| **PR4** | docker-compose + 示例依赖一键启动 | 便捷性 | 0.5–1d | 已完成 |
| **PR5** | architecture / config-reference 文档 | 文档 | 1d | 已完成 |
| **PR6** | resilience 接入 Framework 拦截器链 | 架构 | 2–3d | 后续 |
| **PR7** | 剥离 grpcep 业务校验 + 配置双开关收敛 | 架构 | 2–3d | 后续 |
| **PR8** | 测试补齐（gerr/validation/redis）与覆盖率门槛 | 质量 | 2–3d | 后续 |
| **PR9** | 可选：依赖模块拆分 / 最小无依赖示例 | 架构 | 3–5d | 后续 |

本批次完成 **PR1–PR5**（可合并为一个大提交或按 PR 分 commit）。

---

## PR1 — 仓库卫生

### 目标

- 修正 `.gitignore`，不再误忽略整个 `example/`
- 清理 `.bak`、本地二进制等构建产物
- 约定 ignore 规则（bin、日志、IDE、缓存）

### 变更文件

- `.gitignore`（重写）
- 删除（若存在）：`example/**/bin/*`、`*.bak`

### 验收

- `git check-ignore example/simple/README.md` 无输出（不被忽略）
- `git check-ignore example/**/bin/xxx` 被忽略
- 工作区无 `.bak`、无大体积二进制

---

## PR2 — Makefile + CI + toolchain

### 目标

- 根目录 `Makefile`：`test` / `vet` / `tidy` / `example-simple` 等入口
- GitHub Actions：固定 Go 版本跑 `go test ./...`（排除 example 中需外部依赖的包可选）
- `go.mod` 增加 `toolchain` 说明，降低 1.25.4/1.25.5 混用失败率

### 变更文件

- `Makefile`
- `.github/workflows/ci.yml`
- `go.mod`（toolchain 行，尽量小改）

### 验收

- `make test` 在干净环境通过（或明确跳过需外部服务的集成包）
- CI workflow 语法合法、步骤完整

---

## PR3 — 文档入口

### 目标

- 重写 `README.md` / `README_zh.md` 为能力地图
- 修正 `logger/README.md` import 路径与 API 签名（sprintf 风格）

### 变更文件

- `README.md`
- `README_zh.md`
- `logger/README.md`

### 验收

- 新用户从根 README 能看到全部子包能力
- logger 示例可复制编译（路径正确）

---

## PR4 — docker-compose 与示例脚本

### 目标

- `docker-compose.yml`：etcd + Jaeger(OTLP) + redis + mysql（profile）
- 根 Makefile 或 `scripts/` 增加 `deps-up` / `deps-down`
- 更新 simple / framework 文档链接到 compose

### 变更文件

- `docker-compose.yml`
- `Makefile`（补充 targets）
- `example/simple/README.md`、`example/framework/QUICKSTART.md`（轻量补充）

### 验收

- `docker compose up -d etcd` 可启动（本地有 Docker 时）
- 文档写明端口与用途

---

## PR5 — 架构与配置参考

### 目标

- `docs/architecture.md`：组件关系、启动顺序、请求链路
- `docs/config-reference.md`：YAML 字段参考（从 example 提炼）

### 变更文件

- `docs/architecture.md`
- `docs/config-reference.md`
- 根 README 链到上述文档

### 验收

- 文档与当前代码字段名一致（抽查 tracing.otlp、grpcServer.etcd 等）

---

## PR6+（后续，本批次不实现）

详见评审报告中的第三、四阶段：resilience 接入、业务校验剥离、模块拆分、OTLP 默认化、覆盖率等。

---

## 合并策略建议

1. **内部快速推进**：PR1–PR5 可单分支一次合入，commit 按 PR 标题拆分。  
2. **对外开源 / 严格 review**：每个 PR 单独分支与 PR 描述，附验收 checklist。  
3. 已有未提交的 `grpc_client` 相关改动应**单独 PR**，不与本文档卫生改动混在一起。
