# vela 当前版本

当前版本运行在 Windows 上，使用 Go 1.27.4 和 MyGo 0.8.3，模块最低要求 Go 1.27.3。启动后可在 vela 和 sandbox 两个 Mock 项目间切换。

## 初始化 Git 仓库

点击左上角主菜单，选择 **Git → Initialize Git Repository…**。选择已有文件夹，填写初始分支，默认是 `main`。初始化保留目录中的文件，并拒绝重复初始化或在现有仓库内创建嵌套仓库。

也可在目标目录运行已构建的程序，或在源码目录使用以下命令初始化指定位置。

```powershell
go run ./cmd/vela init -branch main 'F:\path\to\project'
```

## 浏览本地 Git 仓库

从顶部仓库选择器或 **Git → Open Local Repository…** 选择工作树中的文件夹。可在当前会话切换已打开的仓库。Commit 和 Git Log 的刷新按钮会重读仓库状态。

- Project 显示跟踪文件和未忽略的未跟踪文件，编辑器以只读方式打开实际文件。
- Commit 显示工作区和暂存区。可分别查看工作区与 Index、Index 与 HEAD 的文本 Diff。
- Git Log 读取最多 10,000 条提交及其变更路径，展示提交图、分支、标签和详情。历史变更以第一父提交为基准，文件历史从已加载的提交中按当前路径和重命名记录的原路径筛选。
- Manage Remotes 可查看真实远端地址。二进制、非 UTF-8 或超过 2 MiB 的文件会显示预览不可用的原因。
- 在 Commit 区选择一个或多个文件，通过工具栏或右键菜单执行 **Stage Changes** 或 **Unstage Changes**。工作区列表也可使用 `Ctrl+Alt+A` 暂存。暂存读取磁盘上的当前版本，取消暂存保留工作区文件。同一个文件同时存在两种变更时会提示确认。
- 在 Commit 区输入提交信息后点击 **Commit**，提交整个 Index。勾选 **Amend last commit** 可修改本地 HEAD 的消息，并纳入已暂存的内容。确认窗口会显示仓库、分支和要替换的提交。
- 提交使用本机 Git 身份、hooks 和签名配置。Amend 会拒绝本地远端跟踪分支已包含的提交，判断基于本地引用。两种操作都保留未暂存改动，失败时保留消息草稿。
- **Commit and Push…** 先创建本地提交，再打开 Push 对话框。取消或推送失败后，提交仍保留在本地。

## 分支与远端

- **New Branch…** 从当前提交、分支、标签或历史提交创建本地分支，可勾选创建后切换。**Checkout** 可切换本地分支，或从远端跟踪分支创建本地跟踪分支。切换前需提交或收好跟踪文件的改动。
- **Fetch** 从所选远端刷新远端跟踪分支。**Pull** 先获取所选远端分支，再将它快进到当前分支。
- **Update Branch** 按所选本地分支的 upstream 获取并快进。Git 菜单中的 **Update Project…** 以当前分支为目标。更新非当前分支时保留当前工作树，目标分支已在另一个工作树中检出时会拒绝更新。
- **Push** 明确显示源分支，允许指定远端、目标分支和是否设置 upstream。只推送指定分支。Pull 和 Update 遇到分叉会保留本地分支，Fetch 已成功的远端跟踪引用可用于后续检查。
- Update Branch 的行为已用 Git 集成测试验证。与目标 IDEA 版本的行为对照尚未实测。

## 合并与冲突

- 从 **Git → Merge…** 选择来源分支或标签，也可从分支右键菜单发起。合并前需处理跟踪文件的未提交改动。Git 可以快进时直接快进，其余无冲突的合并会创建合并提交。
- 合并暂停时，Commit 区显示未解决的文件。单击文本冲突文件，或右键选择 **Resolve in Merge Editor**，可在左侧 **Current**、中间 **Result**、右侧 **Incoming** 三栏中解决冲突。Result 从 Git 已生成的工作区内容开始，保留已自动合并的内容。
- 用两侧箭头采用当前冲突块，或直接编辑 Result。F7 和 Shift+F7 切换冲突块。清除冲突标记后，点击 **Apply Changes** 或按 Ctrl+S 写入并暂存。若磁盘内容在编辑期间变化，应用会被拒绝。关闭编辑区后，可从 **Merge Drafts** 找回本会话的未应用草稿。
- 文件菜单仍提供整文件采用一侧的操作。在外部编辑后可使用 **Mark Resolved** 暂存文件。
- 所有冲突解决后可点击 **Continue Merge**。**Abort Merge** 会尝试恢复合并前的状态，确认前请保留需要的解决改动。二进制文件不提供文本预览，子模块冲突需在 Git 中处理。

## Rebase

- 从 **Git → Rebase…** 选择基准分支或标签。分支右键菜单也可将当前分支变基到所选引用。确认窗口会提示重放提交可能获得新的提交 ID。
- 暂停时，Commit 区显示正在变基的分支。三栏中的 **Rebased** 是已重放到新基准的内容，**Replayed Commit** 是当前正在重放的提交。应用全部冲突解决结果后点击 **Continue Rebase**。
- **Skip Commit** 会放弃当前提交及该步的解决结果。**Abort Rebase** 会恢复原分支位置。这两项都需要确认。当前界面支持普通变基，含合并提交的重放范围和交互式变基需通过 Git 处理。

## Cherry-pick 与 Revert

- 在真实仓库的 Git Log 中选择提交，点击工具栏的 **Cherry-Pick** 或使用提交右键菜单的 **Cherry-Pick**、**Revert Commit**。确认窗口显示目标分支、完整提交 ID 和提交主题，每次处理一个提交。
- Cherry-pick 将所选变更创建为当前分支的新提交。Revert 创建抵消所选变更的新提交。操作前需要处理跟踪文件的未提交改动；可能被覆盖的未跟踪或忽略文件会受到保护。
- 选择合并提交时，必须明确选择 **Mainline parent**。变更相对于该父提交计算。撤销合并也会影响未来合并能否重新引入原有变更，执行前请确认父提交及其影响。
- 冲突复用三栏编辑器。Cherry-pick 的右栏 **Picked Commit** 是所选提交；Revert 的右栏 **Reverted Parent** 是被撤销提交的选定父版本。中间 **Result** 保留 Git 已自动合并的内容，支持逐块采用、编辑和应用。
- 暂停后可从 Commit 区或 Git 菜单继续、跳过或中止。继续前需要暂存所有冲突的解决结果；Git 保留暂停状态的空结果可选择 **Skip Current Commit**。空 Revert 未留下暂停状态时，会核对并报告未生成提交、HEAD、Index 和跟踪文件未改变，保留 Git 原始诊断。跳过和中止均有确认窗口，会放弃相应的解决改动。
- 当前只从界面发起单个提交。外部序列仍有多个待处理提交时，可在这里解决文件冲突或中止，继续和跳过请使用 Git。

命令语义参考 [Git Cherry-pick](https://git-scm.com/docs/git-cherry-pick) 和 [Git Revert](https://git-scm.com/docs/git-revert)。

## Reset

从 Log 提交右键菜单选择 **Reset Current Branch to Here…**，或从 Git 菜单选择 **Reset HEAD…**。确认窗口显示当前分支和目标完整提交 ID。

- **Soft** 只移动 HEAD，保留 Index 和工作文件。
- **Mixed** 是默认模式，移动 HEAD、重置 Index、保留工作文件，原暂存改动变成未暂存改动。
- **Hard** 同时替换 Index 和跟踪工作文件，需额外勾选丢弃跟踪改动。可能被覆盖的未跟踪或忽略文件会阻止执行。
- 操作会让目标之后的提交离开当前分支。进行中的合并、变基、提交恢复或 Bisect 必须先结束。磁盘内容在确认期间变化会拒绝执行。

## Stash

**Git → Uncommitted Changes → Stash Changes…** 保存暂存和未暂存改动，可填写消息并勾选包含未跟踪文件。忽略文件保留，子模块仓库及存在 assume-unchanged/skip-worktree 索引标记的仓库暂不支持这一操作。Mock Shelf 与真实 Stash 分开。

**Unstash Changes…** 列出真实 Stash。**Apply Stash** 恢复到干净的跟踪工作区，可勾选恢复暂存状态，原条目始终保留。**Drop Selected Stash…** 打开单独的删除确认；没有自动 Pop 或 Clear。

恢复遇到冲突后，可使用三栏编辑器应用解决结果或在外部修改后 **Mark Resolved**，全部解决后创建普通提交。若要放弃这次应用，选中当前 HEAD 并明确执行 Mixed/Hard Reset；Hard 会丢弃跟踪工作文件改动。原 Stash 仍可再次恢复。

## Worktrees

**Git → Worktrees…** 查看目录、提交、分支和锁定状态，**Open Worktree** 在当前应用中打开该工作树。**New Worktree…** 输入不存在的新绝对目录；Detached 模式使用已加载的提交 ID 或完整引用，Branch 模式使用未被其他工作树检出的已有本地 `refs/heads/name`。**New Branch** 模式填写新本地分支名称和起点，一次创建分支及工作树，不替换已有分支，也不自动设置 upstream。分支右键菜单也提供创建入口。创建失败或取消后请检查已刷新的实际工作树和分支；不会自动删除部分结果。

移除必须再次确认，保留分支。主工作树、当前工作树、锁定/缺失工作树不能移除；存在未跟踪、忽略文件或隐藏索引标记时也拒绝。没有 Force 删除。已打开的被移除会话保留缓存标签和草稿，但需要刷新后才能执行 Git 写操作。

## Bisect

**Git → Bisect → Start Bisect…** 输入 Bad 提交和已知 Good 祖先的完整提交 ID 或完整引用。第一版使用 **no-checkout**：当前 HEAD、Index 和工作文件不切换，测试候选版本需在独立工作树或其他外部环境进行。

Commit 面板显示候选 OID；**Show Candidate in Log** 定位已加载的候选提交，可从历史文件菜单打开只读版本。测试后通过 **Mark Good**、**Mark Bad** 或 **Skip Candidate** 明确分类。搜索显示 First Bad，或提示跳过的提交导致结果不可判定。**End Bisect…** 确认后结束搜索。

**Create Candidate Worktree…** 可从 **Git → Bisect** 或 Commit 面板的 **Test Worktree…** 打开。输入新的绝对目录后，把确认框中的精确候选创建为独立 Detached 工作树；候选超出已加载日志时也可使用。模式和 OID 固定，确认期间搜索或候选变化会拒绝创建。随后从 **Worktrees… → Open Worktree** 打开该目录，在外部运行测试，再返回原仓库使用 **Good / Bad / Skip** 标记结果。搜索仓库的 HEAD、Index 和文件保持原位，候选工作树不会自动删除，也不会在下一步自动切换到新候选。

不自动执行测试脚本。外部发起的 checkout 模式或自定义术语搜索请通过 Git 分类；应用可在安全检查后结束。候选超出最多 10,000 条的已加载 Log 时，请根据显示的 OID 使用 Git 检查。

命令语义参考 [Git Reset](https://git-scm.com/docs/git-reset)、[Git Stash](https://git-scm.com/docs/git-stash)、[Git Worktree](https://git-scm.com/docs/git-worktree) 和 [Git Bisect](https://git-scm.com/docs/git-bisect)。

## 工作区

- 点击左上角主菜单可在仓库、分支选择器与横向 View、Git 菜单之间切换。
- Git → Manage Remotes 可在当前 Mock 项目添加、编辑和移除远端，修改保存在本次会话。GitHub、Gitee 和 GitLab 子菜单支持打开公共站点的仓库和复制网页地址。默认远端是用于演示的公开项目。
- 左侧通过活动栏切换 Project 和 Commit，Shelf 位于 Commit 工具窗口的页签中。单击变更文件可预览 Diff，双击打开源文件。示例仓库中的 Stage、Unstage 和提交预览使用 Mock 数据。
- 底部 Git 窗口包含分支树、提交图、历史变更和提交详情。可按分支、作者、日期、路径或文本筛选日志。日期快捷范围以最新加载的提交为基准。
- 编辑器支持标签、右侧拆分和并排或统一 Diff。关闭所有标签后，可从空编辑区打开搜索、最近文件或文件树。
- Settings 提供 Appearance、Editor、Shortcuts、Version Control 和 About。布局和已应用的设置在正常退出时保存。

## 常用快捷键

| 操作 | 快捷键 |
| --- | --- |
| Search Everywhere | 双击 `Shift` |
| Go to File | `Ctrl+Shift+N` |
| Recent Files | `Ctrl+E` |
| Project Navigation | `Alt+Home` |
| Settings | `Ctrl+Alt+S` |
| 显示或隐藏 Git 工具窗口 | `Alt+0` |

## 主界面

[浅色](screenshots/light.png) · [深色](screenshots/dark.png)

截图由当前版本的 MyGo Tester 在 1440 × 900 DIP 下导出。
