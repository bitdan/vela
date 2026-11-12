# vela

vela 是用 Go 和 MyGo 构建的原生 Git 桌面客户端，采用 IntelliJ IDEA 风格的工作区。当前版本支持初始化、浏览本地仓库、暂存文件以及创建和修订本地提交。

## 功能

- 选择目录和初始分支，创建本地 Git 仓库。
- 在同一窗口浏览项目文件、暂存区、工作区、分支、标签、远端和提交历史。
- 创建和切换分支，执行 Fetch、Pull、Push 和 Update Branch。
- 合并分支，在三栏合并编辑器中逐块解决冲突，并继续或中止合并。
- 将当前分支变基到指定引用，处理冲突后继续、跳过或中止。
- 从提交历史执行 Cherry-pick 或 Revert，支持主线父提交选择和冲突恢复。
- 确认后执行 Soft/Mixed/Hard Reset，保存、恢复及独立删除 Stash。
- 创建、打开和安全移除关联 Worktree，手动进行不切换当前工作区的 Bisect 搜索。
- 在多标签编辑器中查看源文件和并排 Diff。
- 双击 Shift 打开 Search Everywhere，查找文件、文本、Go 声明和操作。
- 支持浅色、深色和跟随系统主题，可调整界面缩放与编辑器字体。

## 从源码运行

在 Windows 上使用 Go 1.27.3 或更新版本和 PATH 中的 Git。

```powershell
go run ./cmd/vela
```

[当前版本使用说明](docs/current.md)

## 许可证

[Apache-2.0](LICENSE)
