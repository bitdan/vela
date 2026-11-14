# 默认布局恢复交付记录

日期：2026-11-13。分支：`codex/restore-default-layout`，基线：`a7c1f50`。

## 环境与改动

Go `go1.27.4 windows/amd64`，模块最低 Go `1.27.3`；MyGo 锁定 `v0.8.3`，核对本地模块官方 `go.mod`、`docs/ui/testing.md`、`ui/headless.go` 和 `ui/editor/buffer.go`。没有新增依赖或目录骨架。

新增 `internal/ui/layout_restore_test.go`、本报告、实施计划和截图。修改 `internal/ui/commands.go`、`workspace.go`、`search.go`、`VELA_SPEC.md` 和 `docs/current.md`。

命令 `workspace.restore-default-layout` 由 View 菜单和 Search Actions 共用，恢复五个 Split 比例、四个面板隐藏标记和当前 Commit / Log 工具页。保留主题、偏好、仓库及面板选择、提交草稿、Editor Buffer、光标和拆分状态。仅扩展 UI 命令，沿用 Application 默认值及 Adapter 正常退出保存；无架构依赖、持久化格式或 Git 功能变更。

现有 Main Toolbar、Activity Bar、Commit、Editor、Git 三栏及 Status Bar 区域顺序保持。新增交互为菜单执行和 Ctrl+Shift+P → 输入命令 → Enter；未新增全局快捷键。

## 验证

- RED：新 Tester 测试先因 View 菜单及操作搜索缺少恢复命令而失败；实现后定向测试 PASS。
- `go test ./internal/ui ./internal/adapters -count=1`：PASS，UI 35.891s，Adapters 4.993s。该次设置 VELA_SCREENSHOTS 导出截图。
- `go vet ./internal/ui ./internal/adapters ./internal/bootstrap ./cmd/vela`：PASS。
- `go build -o "$env:TEMP/vela-layout-restore.exe" ./cmd/vela`：PASS。
- 最终命名整理后 `go test ./internal/ui -run 'TestRestoreDefaultLayout|TestSplitterDragAndRestore' -count=1`：PASS（0.275s）；上述 vet、Windows 构建及 `git diff --check` 再次 PASS。
- 独立只读代码审查：无可操作问题。未提交、合并或推送，保留工作开始时的所有未跟踪规范与报告。
- 未运行仓库级测试及 Git Adapter 集成测试；本批未修改 Git 行为。

## 视觉与验收

MyGo Tester 固定 **1440×900 DIP**：[深色](screenshots/layout-restore/dark-restored.png)、[浅色](screenshots/layout-restore/light-restored.png)。两张已实际查看：隐藏面板恢复，Commit / Log 可见，未保存标签和提交草稿保留，区域顺序未改变，无新增弹窗。没有做像素阈值差异比较。

| 项目 | 状态 | 证据与限制 |
| --- | --- | --- |
| 菜单及键盘操作搜索 | PASS | Tester 实际输入、选择与 Enter 执行 |
| 面板比例及可见性 | PASS | 自定义比例、隐藏父子面板、Project/Console → Commit/Log |
| 设置、选择和草稿保留 | PASS | 双主题测试，活动/非活动仓库 Buffer、修改标记、光标、提交草稿及选择 |
| 保存重载、重复恢复 | PASS | 临时 JSON roundtrip 和重复执行 |
| UI 回归、vet、Windows 构建 | PASS | 上述定向命令 |
| 双主题视觉复查 | PASS | 两张固定尺寸截图；非人工桌面签收 |
| 原生桌面启动关闭、人工焦点及 IDEA 实测对照 | UNVERIFIED | 本批未执行 |
| Gate 2 整体签收 | PARTIAL | 等待用户确认，未自行认定通过 |

下一步：用户复核恢复后的双主题主界面及真实键鼠交互，再按 Gate 2 清单补齐人工验收；本批停止在 UI 范围。
