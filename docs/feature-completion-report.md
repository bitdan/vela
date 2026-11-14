# 功能补齐交付记录

实施开始：2026-11-13 23:36（Asia/Singapore），原计划最迟 2026-11-14 04:36 停止新增。会话恢复时已超过原墙钟截止，因此没有启动下一功能批次，仅完成已有代码的必要修正、验证和交付记录。未把空档计为连续工作，也不声称全部未实现项已经补齐。

## 环境与范围

Go `1.27.4 windows/amd64`，模块最低 `1.27.3`；MyGo `v0.8.3`；Git `2.56.0.windows.4`。无新依赖。沿用 `codex/restore-default-layout`、基线 `a7c1f50`，保留原有未提交布局恢复、规范、参考图、报告及测试。未提交、推送、合并或发布。Git 集成测试只操作临时仓库及临时 bare remote。

## 本批实现

1. **真实仓库会话**：最近列表、活动仓库延迟恢复、每仓库提交草稿、独立选择/多选、文件标签身份、固定、光标、滚动和 Diff 模式。缺失仓库或标签保留可重试身份；恢复回调服从用户当前导航。未保存文件内容不持久化。
2. **远端管理**：新增、修改 fetch URL、删除，逐次确认及取消；保护完整配置、include/global origin、跟踪 refs 和独立 push URL。不支持远端重命名、多 URL 编辑或自定义 fetch 映射的删除。
3. **分支与 Tag**：本地安全重命名、删除完全合入 HEAD 且未被工作树检出的非当前分支；固定目标的本地轻量 Tag。不覆盖、不强制删除、不自动推送。
4. **提交检查**：真实 Commit/Amend 应用主题、正文和空行设置；TODO 只检查暂存新增行，失败保留草稿及 Index。
5. **写入取消**：Index/Commit Context 贯通，统一底栏 Cancel Task；任务返回并刷新前保持忙碌，不宣称取消等于回滚。
6. **Clone**：原生对话框、后台执行、目标保护和取消；保留失败目录。切仓库或取消后的成功回调不自动打开仓库。
7. **Blame**：固定 OID 的只读逐行归因及标签恢复，限制 2 MiB / 20,000 行；工作区入口使用 HEAD 已提交内容。
8. **源文件保存**：真实 Project 可编辑及 Save/Ctrl+S，校验 checkout、Index 和磁盘版本，同目录替换、保留 CRLF。保护保存期间的新输入和共享拆分 Buffer；不自动暂存。
9. **Git Hunk**：独立解析、预览、Stage/Unstage、确认后 Discard；写锁下重建版本及预检。首版限跟踪普通 UTF-8 文件的内容修改；丢弃还拒绝 CRLF、转换属性和未保存编辑。刷新匹配身份的干净 source/独立 Split Diff，不覆盖新输入。
10. **文件历史**：独立原生虚拟列表、固定起点、每页 100 条、旧路径、删除 Diff、Enter/右键/版本/复制 OID。保留超出全局 Log 的历史标签身份；最多 100,000 条，长历史性能未验证。
11. **快捷键设置**：13 项工作区命令的编辑、禁用、恢复默认、冲突和 MyGo 保留键验证；草稿隔离、统一分发/提示、JSON 持久化。Double Shift、局部快捷键和多段键位不在首版编辑范围。
12. **Patch 复制与导出**：跟踪 working/staged 或固定提交历史选择，保留二进制、重命名、删除、根提交及 gitlink。固定 Git diff 配置、16 MiB / 100 文件上限，拒绝未跟踪和冲突选择。原生预览最多 256 KiB，原生 Save 对话框保存完整 Patch。文件适配器以同目录临时文件和原子 no-overwrite Link 创建新 .patch/.diff，保护已有文件及 Git 元数据；复制/导出不暂存、提交或记录 Patch 内容。
13. **文件关闭与重载**：真实单标签 Save and Close / Discard and Close / Cancel；保存失败、新输入、取消关闭均保留标签，原仓库回调不抢占新仓库。Discard 只丢弃内存编辑。Reload from Disk 覆盖编辑前确认，保护读取失败、取消及期间新输入，更新共享拆分视图。批量关闭仍跳过脏标签。

## 代码与架构

没有新增顶层模块或改变 IDEA 布局。主要新增文件按边界归属：

| 层 | 新增实现 |
| --- | --- |
| Domain | `internal/domain/blame.go`、`file_history.go`、`patch.go` |
| Application | `internal/application/keymap.go`、`patch.go`；既有 repository/snapshot 增补窄端口与数据 |
| Git Adapter | `internal/adapters/gitcli/remote_management.go`、`ref_management.go`、`clone.go`、`blame.go`、`file_save.go`、`patch.go`、`file_history.go`、`patch_export.go` |
| 文件 Adapter | `internal/adapters/patch_file.go`；既有布局 JSON 兼容新键位配置 |
| UI | `session_restore.go`、`remote_actions.go`、`ref_actions.go`、`repository_clone.go`、`blame.go`、`file_save.go`、`file_refresh.go`、`hunk_actions.go`、`file_history.go`、`keymap.go`、`patch_export.go`、`source_lifecycle.go`，均位于 `internal/ui` |
| 接线 | 既有 `internal/bootstrap/run.go` 后台端口及 RunOnMain 交付；统一 Command Registry |

每项新增对应定向测试。依赖方向仍为 UI → Application 端口 → Adapter，Domain 不引用 MyGo，View 不执行 Git/磁盘 IO。会话与快捷键格式兼容性见 [ADR 0001](adr/0001-real-repository-sessions.md)、[ADR 0002](adr/0002-workspace-shortcut-overrides.md)；详细批次见 [实施计划](superpowers/plans/2026-11-13-feature-completion.md)。产品及架构说明已同步。

## 验证证据

| 命令/检查 | 最新结果 | 范围 |
| --- | --- | --- |
| `go test ./internal/ui ./internal/application ./internal/adapters ./internal/domain -count=1` | PASS，167 tests | 最后关闭/重载防御修正后，2026-11-14 |
| `go test ./internal/adapters/gitcli -count=1 -timeout=20m` | PASS，78 tests | 最新 Git 整包，临时真实仓库及 bare remote |
| `go vet ./internal/ui ./internal/adapters ./internal/adapters/gitcli ./internal/application ./internal/domain ./internal/bootstrap ./cmd/vela` | PASS | 最后关闭/重载防御修正后复验 |
| `go build -o "$env:TEMP/vela-feature-completion.exe" ./cmd/vela` | PASS | Windows 构建，不等于桌面运行验收 |
| `git diff --check` | PASS | 包含文档收尾 |

最新两条 go test 合计 245 tests PASS。未运行仓库级 `go test ./...` / `go vet ./...`；本批按受影响包验证。未做 race、跨平台、性能基准、安装包、发布及原生桌面人工验收。MyGo Tester 覆盖点击、快捷键、选择、确认、取消、错误、新输入和原仓库异步隔离。

独立只读审查提出的会话恢复、完整远端配置、Split Buffer、Clone 排队取消、快捷键 Reset 冲突/原生保留键、刷新身份、gitlink Patch 配置问题均经回归处理。最后的 Mock Reload 崩溃疑虑经补测未复现，审查者已撤回：既有 deferredCommands 已阻断入口，新增 Mock 回归通过；另在底层补充真实 root/Reader/标签防御，不将该疑虑记为已复现 P1。独立审查未留下本批尚未处理的确认缺陷，不代表人工/性能验收已通过。

## 视觉与交互验收

截图位置：`docs/screenshots/feature-completion`，固定 **1440×900 DIP**，MyGo Tester，文件名 `session-*.png`。该目录也含既有截图；本批截图不应混同桌面运行证据。

已复查主窗口、Hunk、保存、Blame、快捷键、文件历史、Patch 导出和关闭/重载代表截图。布局未变，检查到的弹层和操作按钮没有明显裁切。示例：[Dark 主窗口](screenshots/feature-completion/session-dark.png)、[Light 主窗口](screenshots/feature-completion/session-light.png)、[Dark 关闭确认](screenshots/feature-completion/session-source-close-dark.png)、[Light 导出](screenshots/feature-completion/session-patch-export-light.png)、[Light 重载](screenshots/feature-completion/session-source-reload-light.png)。没有完成与指定 IDEA 版本的逐像素差异验收。

原生桌面控制尝试由 computer-use/sky.launch_app 返回 `GetCursorPos failed: 拒绝访问 (0x80070005)` 阻断；后续 list_windows 和进程读取均无测试窗口。没有成功启动或控制本批原生窗口，因此保持 UNVERIFIED。

| 项目 | 状态 | 限制 |
| --- | --- | --- |
| UI/Application/Domain/文件 Adapter 自动验证 | PASS | 167 项通过，限定本批功能边界 |
| 真实 Git 集成验证 | PASS | 最新整包 78 项通过，限 Windows 临时仓库场景 |
| Dark/Light 固定尺寸视觉证据 | PARTIAL | Tester 截图已复查，未做全部 IDEA 差异验收 |
| 桌面键鼠、原生保存对话框实际运行 | UNVERIFIED | 桌面控制被运行环境拒绝 |
| 性能、可访问性、跨平台、安装发布 | UNVERIFIED | 未执行 |
| Gate 2 整体签收 | PARTIAL | 仍待用户确认主界面与交互 |

## 仍未实现或未验证

| 后续优先项 | 当前边界 |
| --- | --- |
| 全局 Log 分页及大仓库性能 | 一次最多 10,000 条；文件历史独立分页不能代替全局分页 |
| 行范围历史、内联 Blame、完整历史图交互 | 当前为文件历史及只读归因文本 |
| Patch 导入/应用、更广 Hunk 类型 | 本批只做导出；新增/删除/重命名/二进制 Hunk 写入仍拒绝 |
| 交互式历史改写 | Edit message、fixup/squash/drop、交互式 Rebase 等仍禁用 |
| 真实 Shelf/Changelist | Mock 展示与 Git Stash 分离，不能视为真实 Shelf |
| 项目文件管理和全局查找替换 | 新建、重命名、删除、剪贴板、Find/Replace in Files 等尚未实现 |
| 文件监听和统一任务管理 | 当前显式刷新及共享取消，不提供后台自动监听/任务中心 |
| 完整键位/部分 Git 设置 | 多段键位、局部键位编辑、AutoFetch/凭据等不能仅因设置可保存而视为生效 |
| PR/MR、更新安装发布 | 未实现或未验收 |

下一阶段优先实际桌面验收，再做固定引用快照的全局 Log 分页及大仓库测量；本批不自动进入下一阶段。实现授权不等于 Gate 签收。

## 后续 PR 交付

用户在实现交付后明确授权提交并创建远程 PR；前文“未提交、推送”描述的是实施交付时状态。本次以 origin/main 为基线提交本批实现、配套规范/ADR/截图及已集成的默认布局恢复。本机 AGENTS.md、无关历史阶段报告/计划和旧截图继续留在本地，不纳入此 PR。提交前重新运行相关包测试、定向 go vet 和 Windows 构建；PR 描述保持桌面验收 UNVERIFIED 与 Gate 2 PARTIAL。
