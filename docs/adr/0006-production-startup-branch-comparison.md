# ADR 0006：正常启动与固定引用分支比较

状态：已实现，功能与视觉验收仍为 UNVERIFIED。

2026-11-18 修正：Compare with 不再显示单个文件差异列表，而直接打开编辑器比较标签，内含上下两组独立 missing-commit Log，固定相同 OID 快照后反转范围，与本地 Rebased 的 `GitCompareBranchesUi.kt` 中正反 range 两个 Log 的行为对照。只参考行为，未复制 Java/Kotlin 实现或资源。每侧独立分页/图/选择/父提交文件/详情，请求取消与身份校验独立。多个分支对的比较标签分别持有状态，命令携带 tabID；Diff 另开标签，切回比较保留状态，关闭取消请求。比较工具标签当前不跨启动恢复，避免冒充普通文件或 Diff 持久化。完整筛选栏、多选聚合、合并提交按父分组尚未实现；本批不宣称完全对齐。

正常桌面启动不再依赖 Mock。Bootstrap 使用 NewWorkspace，Demo/test 构造器保留旧测试基线。Production 只持久保存真实仓库，忽略旧 Mock 名称；显式 Demo 按需创建并显示标识，退出返回真实会话或 Welcome。初始化成功复用仓库读取入口，导航世代与取消检查防止初始化完成覆盖之后的导航。

分支比较不借用当前 Log 数据：窄 RepositoryComparisonReader 返回 Domain 比较快照。适配器固定 selected 与 HEAD，机器可读 name-status 保留 rename 两路径；完整缺失提交范围使用 HEAD..selected，100 条分页并验证 shallow/grafts 边界。初始缺失提交捕获不运行文件差异枚举，文件数量限制不能限制历史完整性。所有实际 Git 使用现有 Executor，无新依赖、写命令或 Shell 拼接。

工作文件比较覆盖跟踪文件与 Index，排除未跟踪文件；通过现有工作区指纹拒绝变化后的预览。保存 compare 来源身份及 ChangeKind/OldPath，重启只重读可验证的内容；不能通过快照恢复原工作文件字节。固定提交的 History/Blame 从右侧 OID 开始，工作快照归因从当前 HEAD 开始（归因明确为已提交内容）。

比较和缺失提交分别使用独立对话框状态、虚拟列表、请求身份和取消 Context；仓库切换/刷新/写入/关闭后不接受旧结果。主 Log 选择、筛选及提交草稿保持独立。二进制和超限内容沿用只读预览限制，列表与历史查询分别报错；大仓库性能、真实桌面、跨平台及完整视觉对照仍需单独验收。
