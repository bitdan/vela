# vela 当前进展

## 已实现

- 原生 IDEA 布局、深浅主题、可调面板、多编辑器标签、仓库与文件会话恢复。
- Git Log/提交图分页、文件历史、Blame、历史目录与文件浏览。
- 暂存、Commit/Amend、分支/标签/远端、Fetch/Pull/Push。
- Merge/Rebase、Cherry-pick/Revert 与恢复路径、Reset、Stash、Shelf、Worktree、Bisect。
- Project 文件管理、查找/替换预览、Patch 导出与受限导入。
- 后台刷新/AutoFetch/incoming、任务中心、保护分支、Commit/Amend 的 GPG 配置。
- Patch 按文件/完整 Hunk 选择、反向应用、Strip 与目标目录重定位。
- Shelf 按整文件 Unshelve；部分恢复保留完整原 Shelf。
- 双向分支比较直接打开 `Compare: A and B` 标签；独立分页/提交图/选择/文件/详情，合并提交可切换父提交。Diff 另开标签，切回保留状态。两侧分别按提交消息、作者、UTC 提交日期和字面文件/目录筛选完整固定范围，Apply/Clear 后从首条重新分页。

## 尚未完成

| 范围 | 待补齐 |
| --- | --- |
| 分支比较 | Hash/正则/区分大小写等筛选选项、多提交变更聚合、合并提交按父节点分组、比较标签跨重启恢复 |
| Patch | 新增/删除/重命名、二进制、权限、CRLF/转换属性、行内选择、三方导入 |
| Shelf/Changelist | Hunk 级恢复与分组、自动列表策略、Shelve Silently 独立语义 |
| 历史整理 | 消息编辑、Fixup/Squash/Drop、交互式 Rebase、拆分/撤销、行范围历史与内联 Blame |
| 编辑器 | 未保存内容跨重启、本地历史、多窗口、完整标签/快捷键配置 |
| 工作流 | Checkout+Rebase/Update、外部多提交序列恢复、远端改名/多 URL、更多签名场景 |
| 发布与协作 | 应用内 PR/MR、安装包签名、自动更新与发布诊断 |

## 使用边界与验证

Patch 导入目前限既有普通跟踪 UTF-8/LF 文本修改；部分 Unshelve 不裁剪原 Shelf。取消不等于回滚，Update 备份保留。比较标签目前不跨启动恢复。

格式、应用编译及 Application/Git Adapter/UI 测试代码编译通过。按约定未执行功能测试、Git 集成测试或桌面验收；功能与视觉为 **UNVERIFIED**，Gate 2 为 **PARTIAL**。

产品规范见 [VELA_SPEC](../VELA_SPEC.md)，架构见 [ARCHITECTURE](../ARCHITECTURE.md)，布局参考见 [IDEA 图片](idea-example.png)。只维护当前能力与待办，不再按批次保存报告、计划和过程记录。代码与必要文档一起版本管理，AGENTS.md 保留本地。
