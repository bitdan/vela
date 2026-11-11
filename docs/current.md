# vela 当前版本

当前版本运行在 Windows 上，使用 Go 1.27.3 和 MyGo 0.8.3。启动后可在 vela 和 sandbox 两个 Mock 项目间切换。

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
