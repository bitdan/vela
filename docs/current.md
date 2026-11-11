# vela 当前版本

当前版本运行在 Windows 上，使用 Go 1.27.3 和 MyGo 0.8.3。启动后可在 vela 和 sandbox 两个 Mock 项目间切换。

## 初始化 Git 仓库

点击左上角主菜单，选择 **Git → Initialize Git Repository…**。选择已有文件夹，填写初始分支，默认是 `main`。初始化保留目录中的文件，并拒绝重复初始化或在现有仓库内创建嵌套仓库。

也可在目标目录运行已构建的程序，或在源码目录使用以下命令初始化指定位置。

```powershell
go run ./cmd/vela init -branch main 'F:\path\to\project'
```

初始化使用系统 Git。Project、Commit、Git Log 和 Manage Remotes 仍使用 Mock 数据。

## 工作区

- 点击左上角主菜单可在仓库、分支选择器与横向 View、Git 菜单之间切换。
- Git → Manage Remotes 可在当前 Mock 项目添加、编辑和移除远端，修改保存在本次会话。GitHub、Gitee 和 GitLab 子菜单支持打开公共站点的仓库和复制网页地址。默认远端是用于演示的公开项目。
- 左侧通过活动栏切换 Project 和 Commit，Shelf 位于 Commit 工具窗口的页签中。单击变更文件可预览 Diff，双击打开源文件。Stage、Unstage 和提交预览使用 Mock 数据。
- 底部 Git 窗口包含分支树、提交图、历史变更和提交详情。可按分支、作者、日期、路径或文本筛选日志。日期快捷范围以最新 Mock 提交为基准。
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
