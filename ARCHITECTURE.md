# vela 技术架构

本文只维护技术边界和关键约束。产品行为见 [VELA_SPEC](VELA_SPEC.md)，实现范围和未验证项见 [当前清单](docs/current.md)。

## 技术栈与模块

Go + MyGo Native UI + 系统 Git CLI。Go 最低版本与 MyGo 锁定版本以 [go.mod](go.mod) 为准；不引入 WebView 主界面、前端构建链、默认 go-git、未经评审的数据库或插件系统。使用 MyGo API 前核对锁定版本源码和官方示例。

| 目录 | 职责 |
| --- | --- |
| `cmd/vela` | 程序入口 |
| `internal/bootstrap` | 配置、依赖组装、异步端口与窗口生命周期 |
| `internal/domain` | 仓库、引用、提交、文件、策略等纯模型 |
| `internal/application` | 用例、窄端口、输入校验、布局和会话模型 |
| `internal/adapters/gitcli` | Git Executor、机器输出解析、文件与仓库 IO |
| `internal/runtime` | 任务记录、取消和刷新调度 |
| `internal/ui` | 原生视图、状态、统一命令、交互和主题 |
| `internal/branding` | 应用图标资源 |

Domain 不依赖 MyGo、文件系统或 Git。Application 依赖 Domain 并定义端口；Adapter 实现端口；Bootstrap 连接具体实现。UI 只读取状态、接收意图并调用端口，不执行 Git、读写文件或在 View 构建中启动任务。不创建预留空接口、万能 Service 或 EventBus。

## 状态与身份

- 仓库会话按绝对工作树路径区分；Worktree、GitDir、CommonDir 分别保存，不能相互替代。
- 当前分支、所选分支、Working、Staged、Log 提交、历史文件和活动编辑器标签独立管理。
- 每个源文件标签持有 Buffer、光标、滚动和未保存状态；拆分视图可共享同一 Buffer。Diff 展示状态不作为 Git Patch 的执行依据。
- Log 查询固定提交集合、引用标签和祖先边界，分页后重连已加载图边，不用可变分支名拼接后续页面。
- 比较标签分别持有上下两侧分页、选择、文件、详情及筛选草稿。命令携带 tabID，回调检查标签成员、请求身份和仓库；关闭取消请求，重新打开不能接收旧请求结果。
- 比较聚合独立端口在冻结范围内核对所选 OID 的成员关系及真实父节点；连续单父链使用首尾树 Diff，含根链使用按仓库对象格式计算的空树。非连续选择与合并保留逐提交/逐父分组，每个文件绑定其 Diff source。
- 比较筛选由 Domain ComparisonLogFilter 表达、Application 校验。Adapter 在固定范围内筛选后分页；消息/作者由 Application 校验并编译为 Go 正则匹配器（字面模式先 QuoteMeta），支持区分大小写；Hash 按十六进制前缀匹配，路径使用字面 pathspec，日期按 UTC。Apply/Clear 只替换本侧请求，筛选条件进入任务去重键。

## 原生命令与视图

Toolbar、菜单、右键菜单、快捷键和 Actions 共用稳定 Command ID、上下文、可执行条件、禁用原因、执行入口、取消和结果处理。不能按 UI 文案判断业务类型，也不能在按钮中复制 Git 逻辑。

颜色、字体、行高、间距和状态色集中在 Design Tokens；深浅主题共用语义。大型列表虚拟化，树形数据评估 Outline。编辑器使用 MyGo editor.Buffer/State/View；Diff 使用 DiffState/DiffView。比较工具标签不冒充文件 Buffer 或 Diff，文件操作、拆分和会话保存须明确处理其类型。

## Git 执行与写入安全

所有 Git 操作经过同一 Executor，使用 exec.CommandContext、独立参数、明确工作目录和受控环境。禁止 Shell 拼接；stdout、stderr 和退出码分别保留。设置超时及输出上限，错误保留上下文，日志不输出凭据。Windows 为每条 Git 命令创建独立 Job Object，进程以挂起状态加入后再恢复，避免子 helper 逃过绑定；优先通过 PSS_CAPTURE_THREADS 仅捕获目标进程的线程 ID，避免每条命令枚举整个系统的线程；API 或权限不可用时回退 Toolhelp。取消/超时终止本次进程树并等待退出，不按 PID/进程名扫描终止，不影响其他请求。正常完成释放 Job，不终止可合法常驻的凭据 helper。

优先使用 porcelain、固定格式和 NUL 分隔解析状态、日志、路径、引用和 Worktree。不把本地化 stderr 当协议，也不假设一行就是一个文件名。路径解析覆盖中文、空格、换行、重命名、模式、二进制和冲突；超限必须报错。

同一仓库写操作串行化，涉及共享引用时必须考虑 CommonDir。内部锁不能阻止外部 Git；每次写前重新检查仓库身份、HEAD、Index、工作区、操作元数据和目标 ref。保护未跟踪/忽略文件、未保存 Buffer、链接及 Git 元数据，不使用 force 或 reject 静默降级。

历史链路按固定对象读取；canonicalHistory 禁止 replace 对象改变读取语义，并检查 shallow/grafts 祖先边界。工作快照通过版本指纹拒绝过期预览，不能把旧内容当作当前磁盘事实。

Fetch、Pull、Push、Update 为独立操作。Commit 成功与 Push 失败分别报告。破坏性操作展示目标和影响后确认；冲突提供继续、解决、跳过或中止等真实 Git 支持的路径。取消只表示提出请求，不能承诺回滚。

## Patch、Shelf 与文件写入

Git Patch 的解析、预检和写入与 MyGo Diff 显示分离。Hunk 写入重新校验路径、HEAD、Index、磁盘指纹与 Hunk 身份；过期或不支持的类型直接拒绝。导入选择、方向、Strip 和目标目录绑定预检身份；检查与应用使用相同路径映射，拒绝越界及目标碰撞。导入的新增/删除/重命名、二进制和权限变更为不可拆分的整文件单元，普通修改保留 Hunk 选择，Stage/Unstage/Discard Hunk 的支持范围独立。解析同时核对 diff、扩展头和正文端点；重命名两端加入受保护路径集合，反向时翻转现存/缺失角色。Working tree 按磁盘存在性、Index only 按 Index 存在性预检，指纹同时包含目标内容与缺失状态；允许安全创建缺失父目录，拒绝链接、元数据目录及相互覆盖的端点。

导入 Patch 的换行校验在 Application 区分原生 LF 包装和 LF/CRLF 正文，无末尾换行标记不计入内容 EOL。选择及写入保持原始字节。Adapter 校验整个磁盘与 Index 文本的换行一致性，并将实际写入目标的换行与对应文件 Patch 正文绑定；转换属性仍拒绝，检查和写入共享固定 autocrlf=false 的 Git 参数。

二进制 Patch 通过标准库 zlib 和有界 Git base85 解码校验两方向数据，限制 literal、delta 指令、源与结果尺寸；不自行执行 delta。Adapter 使用有界原始磁盘/Blob 读取计算指纹，文本编辑器仍只接收编码补丁。文件模式保留正反两侧身份，Index only 依当前方向复核 Index 模式；Windows 明确拒绝工作区执行位变更，不能把系统无法表达的模式宣称已写入。

Shelf 用私有引用保存 stash-compatible 对象，不复用 refs/stash。独立临时 Index 构造选中暂存/工作树，发布快照后才清理工作路径；私有引用通过 CAS 管理。Hunk 选择复用原生 Patch 读取端口，版本绑定 Shelf OID 与仓库/磁盘身份。混合恢复先从 Base 及整文件子集构造临时树，再在临时 Index 应用显式 Hunk；投影 Index 树等于 Base，完整差异必须属于所选路径。只将投影对象传给 git stash apply，原 Shelf 对象仍用于 CAS；不从原暂存树带入未选内容。部分 Unshelve 保留完整原 Shelf；投影树的完整差异必须是显式选择的子集，防止文件/目录替换带入未选路径。恢复可能影响目标侧重命名路径，须锁定相关源编辑器并刷新全部可变视图。

Project 文件操作先捕获并校验源树及目标身份，拒绝覆盖目标和 Git 元数据。同仓库移动先安全复制再回收源项，删除使用回收站，不降级永久删除。写入占用统一忙碌/取消状态；更新路径时保留 Buffer 和编辑状态。

项目替换只对完整、无读取失败的预览执行，拒绝匹配未保存文档的写入。逐文件校验版本，保护写入期间的新输入；部分成功保留真实结果，不虚构跨文件回滚或 Undo。

## 异步、任务与后台刷新

耗时 Git、IO、解析在后台执行；状态只能通过 MyGo 锁定版本的主线程更新机制交付。UI 不同步等待依赖主线程的 worker。每个请求有 Context、超时及请求/导航身份，切换仓库后旧结果不得安装。

任务记录由 UI 线程管理，保存原仓库、稳定 TaskID、状态、阶段、时间和结果；运行态只持有 Context 与取消入口。最多 32 个活动任务、100 个完成记录，历史仅在内存中保存。任务中心不替代 Executor 写锁或写前校验。

Bootstrap 装饰异步端口，去重键包含完整查询身份的摘要，不持久保存敏感参数。取消、超时和真实完成分别处理；写操作完成后用独立 Context 刷新原仓库，回调结束前不解除忙碌或关闭窗口。进度只报告实际检查点，不伪造百分比。

监听事件仅表示可能变化，经去抖和合并后查询。后台刷新保留会话、选择、已加载历史和编辑状态；相同内容不重置 Buffer。只有活动仓库运行后台策略，空闲不高频轮询。后台 Fetch 使用同一 Executor/锁，不隐式整合分支；incoming 检查针对固定 upstream。网络策略以不可变请求传递，不修改全局环境或 Git 凭据配置。

## 策略、签名与持久化

保护分支、提交检查、网络和签名偏好在 UI 线程复制为不可变请求策略，UI 与 Adapter 共用校验。大文件/名称警告的确认绑定当前 Index 摘要，写前重新计算，不能复用旧确认。保护规则不阻断必要的冲突恢复：Mixed/Hard Reset 仅在目标等于重新校验的精确当前 HEAD OID 时豁免历史改写保护，UI 与 Executor 共用同一 Domain 判断；操作状态、确认、工作区指纹及未保存文档保护仍由既有入口执行。

Commit/Amend 的可选 OpenPGP 配置只保存公开 ID/指纹，通过请求级参数覆盖。未启用时继承 Git 配置；不保存私钥或口令，不修改 Git 配置，签名失败不重试未签名提交。其他工作流沿用 Git 配置。

配置及会话使用现有 JSON，新增字段保持向后兼容，写入采用原子替换或等价安全方式。损坏配置应可恢复默认值；凭据交给 Git Credential Helper、SSH Agent 或系统机制。

会话保存最近仓库、草稿、选择、布局和文件标签身份，Git 状态与磁盘文件内容不是持久化事实。Project 的 EditorDraftSnapshot 单独保存用户输入、编辑前基准、原磁盘版本及换行方式；正文限额和身份在 Application 校验，Adapter 保存前及有界读取后再次校验。启动仅后台恢复上次活动真实仓库；文件按当前磁盘、Index 或固定历史 OID 重读，失败可重试。恢复草稿不写磁盘，当前磁盘内容与原基准分开保留，原版本继续约束 Save。比较工具标签通过独立 ComparisonSnapshot 保存固定 OID、筛选、选择、加载范围与布局，启动后重读；用户导航优先于恢复回调。

缓存键包含工作树和对象版本，写入、HEAD/Index 变化及监听使相关缓存失效。没有数据量与性能证据不引入 SQLite 或复杂缓存。

## 启动与关闭

Bootstrap 加载配置、创建 Adapter/Runtime/UI、连接后台端口和窗口，统一处理日志、Git 检测与资源释放。正常启动使用真实空会话和 Welcome；Demo 显式进入，不能把旧 Mock 数据恢复为真实仓库。

退出先取消后台与活动任务，等待真实回调和必要刷新，再处理未保存文件；不得让 worker 在窗口释放后更新状态。

Keep Drafts and Close 通过窄异步 SaveWorkspaceSession 端口在关闭前完成会话持久化；同目录临时文件 Sync 后原子替换。UI 等待真实回调，取消不承诺撤回已完成的替换；内容变化或保存失败保持窗口。显式放弃从最终快照移除草稿。Bootstrap 在持久保存成功后更新会话基准，避免关闭后重复替换或重复备份损坏配置。

## 验证与文档维护

日常按变更范围检查相关包和调用方。Git 集成使用临时仓库和 bare remote；覆盖成功、失败、取消、冲突、特殊路径及操作前后状态。MyGo Tester 验证选择、标签、焦点、键盘、菜单和禁用状态；视觉验收记录固定窗口尺寸并检查深浅主题、调整/隐藏/恢复和空/加载/错误状态。

只在阶段验收、发布、定向验证不足或用户明确要求时运行仓库级检查。未执行的测试、性能和视觉验证明确标记，不用编译成功替代验收。当前执行限制和状态只维护在 [当前清单](docs/current.md)。

项目说明仅维护 README、VELA_SPEC、ARCHITECTURE 和 docs/current 四份 Markdown；保留 IDEA 参考图与许可证。AGENTS.md 是本地工作规则，不纳入提交。不再新增 ADR、分批报告、计划或过程文档；必要的架构变化直接更新本文。
