# vela

Go + MyGo Native UI 构建的 Git 桌面客户端，使用系统 Git CLI，采用 IntelliJ IDEA 风格工作区。

## 运行

Windows 使用 PowerShell 7，安装 Go 1.27.3 或更新版本，并确保 Git 在 PATH 中。MyGo 锁定 v0.8.3，依赖以 [go.mod](go.mod) 为准。

```powershell
go run ./cmd/vela
```

## 文档

- [当前能力、待办与验证状态](docs/current.md)
- [产品与交互规范](VELA_SPEC.md)
- [技术架构与开发边界](ARCHITECTURE.md)
- [IDEA 布局参考](docs/idea-example.png)

功能代码存在不代表已经完成交互和视觉验收；实际验证状态见当前清单。

[Apache-2.0 许可证](LICENSE)
