# vela

vela 是用 Go 和 MyGo 构建的原生 Git 桌面客户端，采用 IntelliJ IDEA 风格的工作区。当前版本支持初始化本地 Git 仓库，并提供可交互的 Mock 项目用于体验工作区。

## 功能

- 选择目录和初始分支，创建本地 Git 仓库。
- 在同一窗口浏览项目文件、变更、Shelf、分支和提交历史。
- 在多标签编辑器中查看源文件和并排 Diff。
- 双击 Shift 打开 Search Everywhere，查找文件、文本、Go 声明和操作。
- 支持浅色、深色和跟随系统主题，可调整界面缩放与编辑器字体。

## 从源码运行

在 Windows 上使用 Go 1.27.3 或更新版本。初始化仓库需要 PATH 中的 Git。

```powershell
go run ./cmd/vela
```

[当前版本使用说明](docs/current.md)

## 许可证

[Apache-2.0](LICENSE)
