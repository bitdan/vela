# vela --- 技术架构设计

> 文档状态：新项目架构基线\
> 版本：2.0.0-draft\
> 技术方向：Go + MyGo Native UI + Git CLI\
> 架构风格：模块化单体 + Ports and Adapters + Use Case 分层

## 1. 架构目标

比较筛选使用 Domain ComparisonLogFilter 和 Application 统一校验。Adapter 在固定范围内按字面消息/作者、UTC 日期和字面 pathspec 筛选后分页，条件进入任务去重键；消息/作者禁止 CR/LF，避免 Git 拆成多个模式。每侧 Apply/Clear 仅取消并替换本侧请求，保留另一侧及 OID 快照。

2026-11-18 分支比较：Bootstrap 对 committed/missing 模式均通过 `ReadMissingBranchComparison` 捕获固定 OID 与祖先边界，不先枚举两 tip 的文件差异。UI 使用两份 `comparisonSide`，正反调用既有 `ReadRepositoryComparisonCommits`；分页后重连各自图边。每侧提交详情通过已有 `ReadRepositoryRevisionChanges` 后台端口加载，独立请求身份、取消 Context、父提交和文件选择；普通/合并提交的 Diff 使用明确父 OID → 提交 OID，根提交沿用历史 Diff 读取。committed 状态由各自 `tab.comparison` 持有，working/missing 仍由原对话框持有；Command 通过 tabID 路由，回调检查标签成员身份/请求身份/仓库。关闭、重载和切换仓库使旧请求失效；打开 Diff 正常切换编辑器标签，不需要暂停/返回专用入口，也不阻塞后台刷新。比较标签不冒充 code/diff 对象，文件操作与 SessionSnapshot 排除工具标签；当前不跨启动恢复。Domain、Application 端口及 Git 写路径无新增依赖。

vela 2.0 是一次新项目重建，不是把旧 Wails/Vue 代码逐文件翻译成 Go。

本架构优先解决：

1.  UI 与 Git 业务完全解耦。
2.  IDEA 风格的主界面、焦点和交互先稳定，再实现 Git 功能。
3.  Git CLI 操作统一、安全、可取消、可测试。
4.  大仓库和大量 Diff 不阻塞 UI。
5.  所有跨模块状态有明确归属。
6.  不为桌面单体应用引入不必要的微服务、复杂事件总线或插件平台。

## 2. 技术决策

  -------------------------------------------------------------------------------------------------------------
  层次                    选择                                 约束
  ----------------------- ------------------------------------ ------------------------------------------------
  语言                    Go                                   业务逻辑以 Go 实现

  桌面框架                MyGo                                 使用 Native UI，不使用 WebView 页面作为主 UI

  UI 组件                 `github.com/egoist/mygo/ui`          使用 MyGo 原生布局、控件、主题和输入模型

  编辑器                  `github.com/egoist/mygo/ui/editor`   使用
                                                               `editor.Buffer`、`editor.State`、`editor.View`

  Diff 展示               MyGo `editor.DiffView`               展示层与 Patch Engine 分离

  Git 引擎                系统 Git CLI                         Git CLI 是仓库操作的真实执行引擎

  文件监听                MyGo 文件监听能力                    监听事件只触发刷新，不作为 Git 状态事实来源

  配置                    JSON                                 设置、快捷键、布局和会话

  数据库                  暂不引入                             只有缓存/查询需求和基准测试证明必要时再评估
                                                               SQLite

  更新                    MyGo Updater                         在安装包和签名方案稳定后接入

  测试                    Go Test + MyGo Tester                结合临时 Git 仓库做集成测试
  -------------------------------------------------------------------------------------------------------------

### 2.1 明确不使用

-   Wails。
-   Vue / React / Svelte。
-   WebView 作为主界面。
-   Vite、npm、Bun 前端构建链。
-   Tailwind CSS、CSS-in-JS。
-   CodeMirror。
-   默认使用 go-git 替代系统 Git。
-   为尚未出现的需求提前建设复杂插件系统。

MyGo Native UI 自己绘制界面，使用 Go API 构建视图。不要把网页 CSS
的设计方式硬套到 Native UI。

### 2.2 版本管理

-   使用当前 MyGo 官方仓库和文档验证 API。
-   固定 MyGo 依赖版本，不使用不可控的浮动版本。
-   `go.mod` 的 Go 版本必须满足 MyGo 当前版本要求。
-   记录 MyGo 版本、依赖版本和验证日期。
-   升级 MyGo 时先跑最小原生窗口、编辑器、Diff、Tester 和 Windows
    构建验证。
-   不得凭记忆编造 MyGo API；API 以锁定版本源码、官方文档和示例为准。

MyGo 官方参考：

-   https://github.com/egoist/mygo
-   https://github.com/egoist/mygo/tree/main/docs/ui
-   https://github.com/egoist/mygo/blob/main/docs/architecture.md
-   https://github.com/egoist/mygo/blob/main/docs/ui/code-editor.md
-   https://github.com/egoist/mygo/blob/main/docs/ui/diff-viewer.md
-   https://github.com/egoist/mygo/blob/main/docs/ui/testing.md
-   https://github.com/egoist/mygo/blob/main/docs/ui/split.md
-   https://github.com/egoist/mygo/blob/main/docs/ui/list.md

MyGo 当前 Native UI 文档说明：原生界面由 Go 构建，不需要
HTML、JavaScript 或前端构建；Windows 使用 Direct3D 11 渲染，macOS 使用
Metal，Linux 使用 OpenGL。以项目实际锁定版本为准。

## 3. 总体架构

采用模块化单体，不拆微服务。

``` text
┌─────────────────────────────────────────────────────┐
│                    MyGo Native UI                   │
│                                                     │
│  Shell / Layout / Design System / Feature Views     │
│  Search Everywhere / Menus / Shortcuts / UI State   │
└────────────────────────┬────────────────────────────┘
                         │ 调用 Use Case / 订阅 ViewModel
                         ▼
┌─────────────────────────────────────────────────────┐
│                  Application Layer                  │
│                                                     │
│  Commands / Queries / Use Cases / Coordinators     │
│  Task Management / Refresh / Result Validation      │
└────────────────────────┬────────────────────────────┘
                         │ Domain Ports
                         ▼
┌─────────────────────────────────────────────────────┐
│                    Domain Layer                     │
│                                                     │
│ Repository / Branch / Commit / Changes / Diff       │
│ Git Operation / Workspace / Conflict                │
└────────────────────────▲────────────────────────────┘
                         │ implements ports
                         │
┌────────────────────────┴────────────────────────────┐
│                 Infrastructure Adapters             │
│                                                     │
│ Git CLI / Filesystem / Watcher / Persistence / OS   │
└─────────────────────────────────────────────────────┘
```

### 3.1 依赖规则

- 真实仓库会话使用现有 JSON 的可选 Root / ActiveRoot 字段，设计与兼容性见 `docs/adr/0001-real-repository-sessions.md`。Bootstrap 连接读端口后恢复活动仓库；其余仅保留最近身份。UI 通过既有异步 Reader 恢复标签，不保存 Git 状态或文件内容。
- 源文档的 editorDocument 保存书签行号和类型覆盖，与 Buffer 一同共享给拆分视图；原生 GutterMarks、OnGutterClick 和 Select 通过统一命令交互。TabSnapshot 添加可选 Bookmarks / FileType，已加载与延迟恢复路径均克隆切片，恢复时校验行号和语言；Apply 校验仓库、标签、文档和 Buffer 身份。兼容性与边界见 `docs/adr/0003-editor-document-metadata.md`。
- Find in Files 由 Application 的纯文本匹配、限额与 ProjectSearcher 窄端口承载；ProjectFiles 适配器通过标准库 os.Root 读取给定 Project 路径，不遍历文件树或调用 Git。Bootstrap 后台执行并通过 RunOnMain 交付。UI 命令捕获路径/未保存文本快照，显式 Find 启动请求；关闭及导航取消，指针、仓库和 Context 保护回调。结果使用 LF 内容指纹与字节偏移，打开前重读或校验现有 Buffer，不持久化查询或内容。
- 真实 Shelf 复用 RepositoryOperation、Executor 写锁和后台刷新。两个临时 Index 构造选中暂存及工作树快照，后者从前者开始以保留暂存 mode/被强制加入的忽略文件；先发布私有 Git ref，再清理工作路径。事务/CAS 支持回收站、恢复、重命名和永久删除，不操作 refs/stash。ChangelistState 是 Application 会话元数据，深复制可选 JSON 字段，刷新时结合 Working/Staged 的重命名信息归属整文件。格式、回滚及 GC 边界见 `docs/adr/0004-shelves-and-changelists.md`。
- 按文件 Unshelve 复用 RepositoryOperator，无新端口或存储。Application.SelectShelfFiles 验证显式集合与当前条目身份，SelectShelfFiles 请求标记区分空选与旧调用的整条默认。Adapter 在临时 GIT_INDEX_FILE 从原 Base 构造选中 Index/Worktree 树及 stash-compatible 对象；两树相对 Base 的 --no-renames 完整路径集合都必须是选区子集，防止 update-index --index-info 的文件/目录替换隐含删除未选路径。重新校验 checkout/worktree、GitDir/CommonDir 和原 ref OID 后才 apply，失败保留原条目。部分成功不改变原 refs，仅完整应用可 CAS 移到回收站。私有 Shelf 读取/应用采用 canonicalHistory，保持原始对象身份。
- Unshelve 的原生虚拟文件列表与选择状态独立保存在 repositoryOperationDialog，原文件入口默认单项。只允许完整路径，不做 Hunk 或自动重命名闭包。Git 目标侧重命名/冲突沿用其原生行为，故执行前拒绝所有脏源 Buffer、锁定当前仓库所有源编辑器，直到回调恢复只读状态并刷新全部可变视图；不假定只刷新选中来源路径即可。
- CommitWriter 接收可选、不可变的 CommitPolicy；Bootstrap 在 UI 线程复制偏好后传入后台任务。Git Adapter 在串行提交链路中检查暂存新增行，不在 View 中读文件。整文件 Index 和 Commit 回调显式接收可取消 Context，完成后使用独立刷新 Context 读取实际结果，UI 忙碌状态持续到结果交付。
- CommitPolicy 同时携带暂存大文件/名称检查、阈值、保护模式及一次性 WarningConsent。Adapter 持锁对当前 Index 使用 cat-file --batch-check 读取 Blob 大小，名称检查不读工作文件；最多 10,000 暂存项和 100 展示警告。未确认时返回 typed Application.CommitWarnings，UI 保留草稿、通过统一命令显示可滚动原生确认，并把摘要放入新请求 Context；Bootstrap 复制到 policy。每次写前重新计算摘要并校验 HEAD/Index/分支和操作元数据。没有文件 IO 放入 View，没有新增万能检查服务或依赖。
- CommitPolicy.Signing 是 Domain.CommitSigningPolicy 的不可变快照。GitSettings 新增 SignCommits/SigningKey，沿用现有 JSON 默认值及 Settings 草稿 Apply/OK/Cancel；关闭覆盖只继承 Git 配置。Domain 验证公开 ID/指纹，Adapter 仅在 Commit/Amend 启用时添加请求级 gpg.format=openpgp 与 --gpg-sign=<key> 独立参数，在警告/快照/操作校验后执行。没有私钥/口令 IO、Git config 写入、额外进程管理或未签名 fallback；其他 Git 工作流不使用该覆盖。实际签名/口令交互未功能验收。
- Domain.RepositorySafetyPolicy 保存明确的保护模式/CherryPickSuffix，纯函数验证 semicolon path.Match 模式并选择本地历史写操作；Bootstrap 复制 GitSettings，UI 与 Executor 共用 guard。恢复操作不受新增保护阻塞，普通提交/合并/非强制推送保留。选中文件 Cherry-pick 的来源标记使用原始 OID；普通序列使用 Git -x，冲突继续保持 Git 语义。
- Clone 原接口保留，新增 RepositoryNetworkCloner.CloneWithPolicy 窄端口由同一 Executor 实现；只为请求复制 NetworkPolicy，现有 UI 端口和任务中心装饰器不变。ShowPushDialog 的自动路径复用现有 RepositoryOperation/Executor，要求 upstream、RemoteVersion 及实际构造的远端/目标精确匹配原提交快照；变化则回到对话框，不使用刷新后的新 URL 静默推送。

- Remote 管理及 Branch/Tag 管理复用 RepositoryOperation 和现有写锁。真实远端管理使用完整 remote/branch 配置及 scope/origin 的独立版本，避免仅校验传输参数而删除较新的配置。
- 可配置工作区键位由 Application 保存稳定 ID、规范化组合键及有效映射验证，不依赖 MyGo；UI 映射到当前锁定版本的原生 Key/Modifiers，并复用统一 Registry。Layout.Keymap 为可选指针，发布后映射不可变，设置草稿克隆并在 Assign/Reset/Apply 校验；菜单与编辑区提示读取同一映射。JSON、兼容性和原生焦点范围见 ADR 0002。

-   仓库初始化由 Bootstrap 将 `application.RepositoryInitializer` 接到 `adapters/gitcli`。GUI 在后台运行初始化，并在 MyGo 主线程更新状态；成功后复用 RepositoryReader 打开新仓库，取消及导航世代阻止过期结果切换当前仓库。命令行入口复用同一 Git 适配器。

- 正常 Bootstrap 使用 `application.NewWorkspace`，只创建空会话；`NewDemo` 和兼容 `New` 保留确定性测试数据。Production 模式排除空/Demo 快照并拒绝旧 Mock 恢复；UI 的 Welcome 白名单阻止空会话触发模拟仓库操作。显式 Demo 按需分配会话，真实草稿和会话保留，正常选择器不隐式切入 Mock。
- `RepositoryComparisonReader` 是独立窄只读 Port，返回 Domain 的 `RevisionComparison` 与分页提交。Git 适配器通过统一 Executor 固定引用、禁止 replace/外部 diff/textconv，校验 HEAD/Index/工作区和祖先边界；缺失提交捕获不枚举文件树，避免差异文件上限阻断完整历史查询。Bootstrap 后台调用并通过 RunOnMain 回传。
- 比较窗口及分页请求独立于全局 Log，使用 Context、请求身份和仓库身份失效检查。Diff 来源 `compare:<left>:<right>` 或带工作区指纹的 `compare:<left>:working@<hash>`；会话 DTO 复用 ChangeKind/OldPath 保存重命名身份。工作内容变化拒绝旧预览，不持久化内容。详见本地 ADR 0006。

-   `domain` 不依赖 MyGo、Git CLI、文件系统或 UI。
-   `application` 依赖 Domain 和自己定义的 Ports。
-   `adapters` 实现 Application 定义的 Ports。
-   `ui` 只调用 Application 暴露的接口，不直接调用 Git CLI。
-   `bootstrap` 负责创建具体 Adapter、Use Case、Runtime 和
    UI，并组装依赖。
-   `runtime` 提供任务、刷新、缓存和文件监听等运行时能力，不承载 Git
    业务规则。

依赖方向必须朝 Domain 收敛。禁止 UI 直接 import `os/exec` 执行 Git
命令。

## 4. 建议目录结构

``` text
vela/
├── cmd/
│   └── vela/
│       └── main.go
│
├── internal/
│   ├── bootstrap/
│   │   ├── app.go
│   │   ├── dependencies.go
│   │   └── shutdown.go
│   │
│   ├── domain/
│   │   ├── repository/
│   │   ├── branch/
│   │   ├── commit/
│   │   ├── changes/
│   │   ├── diff/
│   │   ├── operation/
│   │   ├── workspace/
│   │   └── conflict/
│   │
│   ├── application/
│   │   ├── ports/
│   │   ├── commands/
│   │   ├── queries/
│   │   ├── services/
│   │   └── viewmodel/
│   │
│   ├── adapters/
│   │   ├── gitcli/
│   │   │   ├── executor/
│   │   │   ├── parser/
│   │   │   ├── repository/
│   │   │   ├── branch/
│   │   │   ├── commit/
│   │   │   ├── changes/
│   │   │   ├── diff/
│   │   │   ├── remote/
│   │   │   ├── worktree/
│   │   │   └── operations/
│   │   ├── filesystem/
│   │   ├── persistence/
│   │   └── platform/
│   │
│   ├── runtime/
│   │   ├── tasks/
│   │   ├── refresh/
│   │   ├── watcher/
│   │   └── cache/
│   │
│   └── ui/
│       ├── app/
│       ├── shell/
│       ├── layout/
│       ├── design/
│       ├── components/
│       ├── commands/
│       ├── state/
│       └── features/
│           ├── commit/
│           ├── branches/
│           ├── history/
│           ├── changes/
│           ├── diff/
│           ├── editor/
│           ├── sync/
│           ├── conflicts/
│           ├── rebase/
│           ├── stash/
│           ├── worktrees/
│           ├── remotes/
│           └── settings/
│
├── assets/
│   ├── icons/
│   └── fonts/
│
├── docs/
│   ├── reference/
│   └── adr/
│
└── tests/
    ├── fixtures/
    ├── integration/
    └── performance/
```

目录可以随着实现调整，但模块依赖规则不能随意改变。不要为了预留而创建大量空目录和空接口；先建立实际需要的边界。

## 5. Domain 模型

Domain 只表达 Git 和工作区概念，不包含 UI 控件、颜色、窗口或 MyGo 类型。

建议核心模型：

``` go
type RepositoryID string
type WorktreeID string
type ObjectID string
type RefName string

type Repository struct {
    ID          RepositoryID
    Root        string
    GitDir      string
    CommonDir   string
    WorktreeID  WorktreeID
}

type Branch struct {
    Name           RefName
    IsCurrent      bool
    Upstream       *Upstream
    Ahead          int
    Behind         int
    IsRemote       bool
}

type Upstream struct {
    Remote string
    Branch string
}

type Commit struct {
    Hash      ObjectID
    Parents   []ObjectID
    Author    string
    Email     string
    Timestamp time.Time
    Subject   string
}

type FileChange struct {
    Path       string
    OldPath    string
    Status     ChangeStatus
    Staged     bool
    Unstaged   bool
    IsBinary   bool
    IsConflict bool
}
```

这是概念示例，不要求逐字照搬。实现前应根据 Git CLI
的真实输出和使用场景调整。

### 5.1 关键区分

-   Repository 与 Worktree 不是同一个概念。
-   GitDir 与 CommonDir 不是同一个概念。
-   CurrentBranch 与 SelectedBranch 不是同一个概念。
-   Working Tree 与 Index 不是同一个概念。
-   Working Changes 与 Historical Changed Files 不是同一组数据。
-   Diff 展示状态与 Git Patch 执行状态不是同一个概念。

多个 Worktree 可以共享 CommonDir，但有独立的工作目录和
HEAD。仓库会话、写操作锁和缓存键必须明确使用哪一个身份。

## 6. Application Layer

Application 负责把用户意图变成可执行的 Use Case。

### 6.1 Commands 与 Queries

Query 只读取，不修改仓库：

-   `GetRepositoryStatus`
-   `ListBranches`
-   `ListTags`
-   `ListRemotes`
-   `GetCommitLog`
-   `GetCommitDetails`
-   `GetChangedFiles`
-   `GetDiff`
-   `GetFileHistory`
-   `GetBlame`
-   `ListWorktrees`

Command 负责执行明确的写操作：

-   `StageFile`
-   `UnstageFile`
-   `StageHunk`
-   `UnstageHunk`
-   `Commit`
-   `AmendCommit`
-   `CheckoutBranch`
-   `CreateBranch`
-   `Fetch`
-   `Pull`
-   `UpdateBranch`
-   `Push`
-   `Merge`
-   `Rebase`
-   `CherryPick`
-   `Revert`
-   `Reset`
-   `Stash`
-   `CreateWorktree`
-   `RemoveWorktree`

每个 Use Case
应有明确输入、输出、错误和副作用，不要建立一个包含所有操作的万能
`GitService`。

### 6.2 Ports

Ports 应以业务能力命名，不要直接暴露 `exec.Command`。

Phase 2 的 `RepositoryReader` 读取仓库快照和单个文件内容。快照包含状态、项目路径、引用和首批最多 500 条提交的元数据，以及固定 HEAD/branch/remote/tag commit OID 的 CommitLogSnapshot。`RepositoryLogReader` 从同一不可变引用快照按 offset 每页读取最多 500 条，以 limit+1 判断后续记录，不设 10,000 条总量截断；通过 stdin 传递固定 OID，单页 stdout 限 16 MiB。禁用 replace refs，读取前后校验 shallow/grafts 指纹；边界变化、对象消失或超时均显式失败。源文件和 Diff 的两侧内容按需读取。Bootstrap 在后台执行查询，通过 MyGo 主线程回调交付结果。工作区用请求身份和取消上下文隔离切换、刷新和预览。

全局 Log 的 Load More/Retry/Cancel 复用 Command Registry；View 构建不触发读取。成功追加复制已有 Commit 结构，Domain.ReconnectCommitLog 按原始父 OID 重建已加载父索引和 lane，UI 只重建 Graph/筛选派生数据，保留原 ListState、选择及 Editor Buffer。切仓库、刷新、写入、关闭会取消并失效旧请求。筛选明确只作用于已加载记录；再次刷新捕获新引用并回到首批 500 条，不持久化分页数据或快照。

`RepositoryCloner` 在统一执行器下创建新目录并克隆，不自动清理失败目标。`RepositoryBlameReader` 读取固定对象的机器协议输出并构建 Domain.BlameDocument；UI 格式化只读文本，不参与 Git 解析。`RepositoryFileWriter` 接收已读取文件的 DiskVersion 和提交文本，复用 checkout 校验及写锁，通过根目录约束的同目录临时文件替换保存，保留权限和行尾；二次校验防止覆盖外部修改。UI 使用共享 Buffer 的原生编辑器、Save Command 和可取消回调，并保留后台保存期间的新输入。

`RepositoryPatchReader` 提供 Domain.PatchDocument，Application.ParseTextPatch 仅解析受支持的 Git 单文件文本补丁。UI 的 Hunk 弹层读取和预览该文档，与 MyGo.DiffState 的视觉差异算法完全独立。写请求只传路径、版本、hunk ordinal 和操作类型，不接受 UI 拼接的 Patch。执行器在写锁下重建版本、预检并执行 git apply；Index 操作不读取或修改无关工作区二进制内容。完成后由读端口刷新已打开的干净 source/Diff Buffer；原仓库、请求身份和 Buffer 身份约束防止导航抢占或覆盖新输入，下一次写入/退出取消旧刷新。

`RepositoryFileHistoryReader` 按固定起点提供 Domain.FileHistoryPage，每次查询 --follow 的有限前缀再截取页面，避免 --skip 跳过重命名转换。UI 使用独立虚拟列表和请求身份，不替换全局 Log；前缀有 offset 上限，UI 限 100,000 条。固定修订源 `revision:OID` 直接读取 blob，不要求父提交或当前路径存在。会话的历史标签仅增补不可变 ChangeKind/OldPath 身份，复用 ADR 0001 的向后兼容 JSON。

`RepositoryRevisionTreeReader` 提供固定 OID 的直接目录子项及显式主线父提交的 Changed Files；Bootstrap 后台调用并 RunOnMain 交付。Domain.RevisionTree/Entry 不依赖 MyGo；UI 使用虚拟列表、可取消请求和独立选择，合并比较不复用第一父提交的缓存列表。历史检出、工作区恢复和所选差异提交通过既有 RepositoryOperation/Executor 写锁串行执行。选中差异以临时 Index 与 commit-tree 构造内部提交，再调用既有 Sequencer，不生成用户分支或私有长期 ref。Context 限定的 canonicalHistory 标记使执行器对历史链路追加 GIT_NO_REPLACE_OBJECTS=1，文件读取亦使用原始对象，防止与 Log 身份不一致；不创建全局可变执行状态。Windows 别名和工作文件保护在写入前完成。详见本地 ADR 0005。

`RepositoryPatchExporter` 通过统一执行器读取 Domain.PatchExport，固定 diff 参数、字面路径和 stdout 上限，校验选择身份及读取前后快照。`PatchFileWriter` 的文件适配器约束真实目录和 Git 元数据，通过同目录临时文件及 Root.Link 创建全新的目标，不覆盖或删除用户文件。Bootstrap 后台处理原生 Save 对话框、文件写入和原仓库刷新，再由 RunOnMain 交付；UI 只持有预览及请求状态，复制结果不记录 Patch 内容。预览截断与实际导出长度独立。

`RepositoryPatchImporter` 是独立窄端口，Domain.ImportedPatch 保存受限文本、路径/mode、Hunk 数和目标专属版本。Application.ParseImportedPatch 验证多文件 Git 头一致性，复用 ParseTextPatch 的 Hunk 计数与换行标记规则；不执行 IO。Git Adapter 有限额读取原生选择器提供的外部普通文件，验证实际目标路径、转换属性、Index flags、mode、元数据目录身份和 checkout/worktree/磁盘版本；预览及应用均使用现有 Executor 写锁。应用重新解析捕获原文，在检查前后重复预检，使用固定 git apply 参数，不追加第三方依赖、不改变持久化格式。Bootstrap 后台执行 picker/preview/apply，RunOnMain 交付，并以独立超时刷新 Context 读取原仓库。UI 统一命令注册、确认与目标选择，切仓库/关闭取消并失效待预览请求；写入复用 repositoryWrite 取消/关闭保护，临时锁定目标源编辑器，完成后恢复并刷新关联视图。取消/失败不保证跨文件事务回滚，明确报告实际结果可能已写入。

Patch 的部分/反向导入在既有普通文本范围内增加：Domain.ImportedPatch 包含解析后的 Files/Hunks 与 Reverse；Application.SelectImportedPatch 从捕获原文重新解析，按稳定路径/Hunk 序号选择并保留精确 Header/Text，拒绝空/非法身份，最多 10,000 Hunk。UI 保留不可变原输入与独立 bool 选择，原生虚拟列表通过统一 Command 更新，选项改变立即使候选版本/同意失效。SelectParsedImportedPatch 只组装已捕获 Header/Hunk 与无权限的候选元数据，避免在输入目录时重复解析原文；后端入口仍完整重解析。RepositoryPatchReviewer.ReviewImportedPatch 是新增窄端口（原 Importer 接口不变），Adapter 忽略来访 Files/Modes/Version、只重解析 Text，再校验并签发目标版本；Bootstrap 后台交付并接入任务中心。Reverse 同时进入 apply --check、实际写参数及版本摘要。应用重新解析被确认文本，重复现有路径/元数据/转换属性/Index/磁盘校验，无持久化或新依赖；Strip 为 1–16（默认 1），Directory 为仓库相对目录；ResolveImportedPatchPath 实现先 Strip 后 prefix，RelocateImportedPatch 在后台从原文派生权威目标 Paths/Modes，再进入原有真实路径/元数据/别名/属性检查。应用与预检传同一 -pN/--directory 参数，映射选项进入摘要和任务去重身份。UI 显示源 → 目标，Buffer 检查/锁定/刷新均使用目标路径；RelocateParsedImportedPatch 仅用于无权限 UI 候选，强制清空 Version。无行内/三方/新增文件扩张。

Project 单标签关闭和重载由 sourceFileDecision 绑定 root、tabID、Buffer，复用现有文件端口。保存完成只关闭同一干净 Buffer 的原标签；重载以读取前文本及请求身份保护新输入。导航取消未完成重读，保存回调依原仓库处理，批量关闭保护未保存标签。

Phase 3 的 `IndexWriter` 接收仓库快照和选中的整文件变更。Git 适配器在串行写入前核对仓库根、分支、HEAD、索引指纹和所选路径状态，以 NUL 分隔的字面路径调用 Git。UI 按仓库根更新操作后的快照，保持正在浏览的仓库不变。

`CommitWriter` 复用 checkout 校验和写入锁，核查 Index 冲突与进行中的 Git 操作，并以 stdin 传递提交信息。Amend 额外检查本地远端跟踪分支的可达性。Git 原生 hooks、身份和签名继续生效。

`RepositoryOperator` 通过明确的操作类型处理分支与同步。远端操作验证配置指纹，Pull 与 Update 在 Fetch 后重新核对 checkout，非当前分支通过带旧 OID 的引用更新做快进。后台完成后刷新原仓库，Fetch 已成功但集成失败时保留阶段信息。

`RepositoryUpdater` 是 Update 专用窄端口，返回 Domain.UpdateResult 的 Fetch/Cleaned/Integrated/Restored 与备份身份，不把已完成的部分阶段压缩成单一成功/失败。Domain.RepositoryOperation 保存明确 UpdateMethod/UpdateCleanup/ConfirmUpdateCleanup；UI 在确认时捕获默认偏好，Bootstrap 复制网络策略后后台调用并用独立刷新 Context 回读原仓库。默认操作仍复用既有 Update Command ID；RepositoryOperator 收到显式策略时也走相同的私有持锁实现，旧空策略调用保持快进兼容路径。

Git Update 在现有 Executor 写锁内先 Fetch、固定目标 OID，再校验 checkout/元数据目录/upstream 与工作区。Stash/Shelf 保存复用已实现的窄持锁逻辑，备份使用唯一名称，取消或清理错误后仍独立限时读取发布身份。Shelf 使用既有私有 refs，Stash 使用 Git 自身 reflog，不新增持久化格式或自动恢复标记。集成成功才尝试 --index 恢复，备份永久保留供用户决定删除；失败或暂停不自动应用备份，不模拟事务回滚。非当前分支固定 OID 快进/CAS，不切换工作树。UI 锁定 Project 编辑器，拒绝未保存内容，恢复/失败均刷新原仓库与关联视图；Merge/Rebase 恢复命令同时增加未保存内容保护与可变视图刷新。

Merge 复用同一执行器和写锁。快照从工作树的 `MERGE_HEAD` 与索引阶段读取合并和冲突状态。三栏编辑器读取两侧对象和工作区结果，各仓库独立保留会话草稿。应用时适配器重新验证 HEAD、Index、合并目标、文件冲突条目和原工作区文本的指纹，再替换工作文件并暂存。替换成功而暂存失败时分别报告。

普通 Rebase 读取每个工作树的 `rebase-merge` 元数据，保存原分支、旧 tip、新基准和暂停步骤。继续、跳过、中止和冲突写入复用 checkout 校验，并检查步骤和原分支引用。开始时关闭自动暂存和其他引用的自动更新，并检查可能被覆盖的未跟踪路径。

Cherry-pick / Revert 由同一 RepositoryOperator 串行执行。快照读取工作树的 CHERRY_PICK_HEAD、REVERT_HEAD 和 sequencer 恢复文件，保存步骤身份与元数据指纹。继续、跳过、中止及冲突写入重新核对 checkout 与步骤，编辑草稿也绑定该身份。原始父提交 OID 独立于可见 Graph 的父节点索引，避免日志截断影响主线父提交选择；适配器再校验所选父序号。每次仅发起一个提交，外部序列仍有多个待处理提交时，继续、跳过交给 Git。执行前保护未跟踪和忽略路径，非交互编辑器环境避免后台等待外部消息编辑器，原生 hooks 与签名配置仍生效。

Git 对空 Revert 可能不保留恢复元数据。适配器只在 Git 退出码为 1、上下文未取消、checkout 指纹未变、跟踪文件干净且不存在进行中操作时，返回 ErrRevertUnchanged，并包装原始诊断；UI 报告未生成提交，不伪造暂停状态或解析 stderr 文案。

Phase 4 的 Reset、Stash、Worktree 和手动 Bisect 扩展同一 `RepositoryOperator`，不引入新的通用 Service。确认对话框持有打开时的仓库快照，而非在确认时重新捕获已变化的目标。Bootstrap 沿用后台执行和主线程结果交付，所有路径与引用作为独立 Git 参数。

仓库快照新增 GitDir/CommonDir、Stash 列表及版本、Worktree 列表及版本、BisectState 和磁盘内容指纹。指纹在后台读取原始跟踪文件和未忽略的未跟踪文件，包含路径、文件模式、符号链接目标及删除状态；读取过程检查 Context。原始内容校验不依赖 Git Diff，因此不会被 assume-unchanged、skip-worktree 或 clean filter 隐藏。大型仓库全文件指纹的成本尚需性能测量，暂不引入缓存或数据库。

Stash 列表使用 NUL 分隔的 OID、selector 和 subject，整个列表产生版本指纹；应用使用 OID，删除重新核对 selector。未合并索引在没有 MERGE_HEAD 的情况下仍可进入冲突编辑器，以已验证的 HEAD、Index 阶段和文件指纹为身份，且拒绝新增的其他 Git 操作元数据。

Worktree 使用 `list --porcelain -z` 解析，读写均区分各工作树的 GitDir 和共享 CommonDir。Executor 的现有全局写锁保守地串行化该实例所有仓库的写入，也覆盖同 CommonDir 的多个工作树；不改变锁粒度。Windows 路径身份比较结合规范路径与文件身份，兼容长短路径别名。移除前额外枚举包含忽略文件的未跟踪路径，并拒绝隐藏索引标记，不仅依赖 Git 的非 Force 删除。应用内锁不能消除外部工具在最终校验与 Git 命令之间的竞争。

New Branch 模式用单个 `worktree add --no-track -b` 创建新分支及关联检出，不先单独改写分支引用；重新检查分支名称和已存在引用，不提供 `-B` 或 Force。Git 在执行中失败或取消可能留下部分创建结果，仍刷新实际引用和工作树列表，不自动删除可能含用户数据的目录或分支。

Bisect 从各工作树的 BISECT_START、BISECT_HEAD、日志、术语及 refs/bisect 读取状态并生成步骤版本；状态与普通 HEAD、Log 选择分离。只发起 no-checkout 搜索，使用稳定候选 OID 分类，借助机器可读引用和 rev-list 计算唯一或不可判定结果。外部 checkout 和自定义术语搜索仅提供受保护的结束路径。其他改变分支历史的命令在 Bisect 期间禁用。

候选工作树复用 CreateWorktree Operation，在请求中携带 `BisectVersion` 和固定候选 OID。搜索版本包含 BISECT_START 的写入时间，区分普通结束后重新启动的相同范围。适配器在现有锁内复核快照身份、实时搜索版本和候选；仅受支持的 no-checkout 搜索可用。通过该身份校验后，才允许读取未出现在 UI History 中的候选对象，不放宽通用 `resolveSource` 对任意 OID 的限制。UI 只收集目录和发出命令，不读取元数据、不直接调用 Git。

概念示例：

``` go
type RepositoryReader interface {
    Status(ctx context.Context, repo Repository) (RepositoryStatus, error)
    Branches(ctx context.Context, repo Repository) ([]Branch, error)
    CommitLog(ctx context.Context, repo Repository, q CommitLogQuery) (CommitPage, error)
    CommitDetails(ctx context.Context, repo Repository, id ObjectID) (CommitDetails, error)
}

type RepositoryWriter interface {
    Stage(ctx context.Context, repo Repository, paths []string) error
    Unstage(ctx context.Context, repo Repository, paths []string) error
    Commit(ctx context.Context, repo Repository, input CommitInput) (ObjectID, error)
}
```

实际接口应按 Use Case 和测试需要拆分。不要过早创建庞大接口，也不要让
Domain 依赖具体 Git CLI 结构体。

## 7. Git CLI Adapter

### 7.1 统一 Executor

所有 Git 命令通过统一 Executor 执行。

``` go
type CommandRequest struct {
    RepositoryPath string
    Args           []string
    Timeout        time.Duration
}

type CommandResult struct {
    Stdout  []byte
    Stderr  []byte
    ExitCode int
}
```

要求：

-   使用 `exec.CommandContext`。
-   禁止通过 `cmd /c`、PowerShell 或 Shell 字符串拼接 Git 命令。
-   命令参数必须作为独立参数传递。
-   仓库路径、文件路径和 ref 不得通过字符串拼接成命令。
-   stdout、stderr、exit code 分开保存。
-   支持 context 取消和超时。
-   对需要持续输出的命令支持流式读取和进度解析。
-   统一设置工作目录和必要的环境变量。
-   错误中保留可诊断信息，但不得泄漏凭据、Token 或敏感环境变量。
-   Git 可执行文件路径可配置，并提供版本检测。
-   任何命令都必须有明确的仓库上下文。

### 7.2 机器可解析输出

优先使用 Git 的机器可读格式，不解析面向人的本地化文本。

重点关注：

-   `git status --porcelain=v2 -z`
-   `git log` 的自定义稳定格式及适当的 `-z` 分隔。
-   `git diff --no-ext-diff` 和必要的原始路径信息。
-   `git for-each-ref`。
-   `git worktree list --porcelain`。

Parser 必须覆盖：

-   UTF-8 中文路径。
-   空格、制表符和换行符。
-   引号、反斜杠和特殊字符。
-   Rename / Copy。
-   文件模式变化。
-   冲突状态。
-   空文件和二进制文件。
-   大量输出和截断风险。

不要假设一行就是一个文件名。对 `-z` 输出按 NUL 字节解析。

### 7.3 Git 写操作协调

同一仓库的写操作必须串行化，避免多个 UI 动作同时改写 Index、HEAD 或
refs。

建议按 CommonDir
建立写操作协调器；如果后续证据表明某些操作可以更细粒度并发，再拆分锁范围。

注意：应用内部锁不能阻止外部 Git、IDE
或用户终端修改仓库。每个写操作执行前仍必须重新读取并校验关键状态。

### 7.4 Update / Pull / Fetch / Push

四者必须是独立 Use Case。

-   Fetch 不隐式更新工作区。
-   Pull 明确 remote、branch 和集成策略。
-   Update Branch 以选中的本地分支和 tracking 信息为上下文。
-   Push 明确目标 ref 和 upstream。

Update 的具体行为必须通过目标 IDEA
版本实测后写入产品规范。没有证据时不要猜测其 Merge / Rebase 策略。

### 7.5 Patch Engine

Diff Viewer 和 Patch Engine 分离：

``` text
Git Diff Output
      │
      ▼
Diff Parser
      │
      ▼
DiffDocument
  ├── FileChange
  ├── Hunk
  └── DiffLine
      │
      ├── UI: MyGo DiffView
      │
      └── PatchBuilder
              │
              ▼
        Patch Validation
              │
              ▼
        Git Apply Adapter
```

Patch Engine 负责：

-   生成指定文件或 Hunk 的 Patch。
-   保留文件路径和必要的 Header。
-   处理行号和上下文。
-   对过期 Patch 做校验。
-   预检 Patch 是否可应用。
-   支持 Stage、Unstage 和 Discard 的不同方向。
-   明确拒绝不支持的二进制或复杂 Diff 场景，而不是静默执行错误操作。

Stage Hunk、Unstage Hunk 和 Discard Hunk 必须分别测试。不能仅凭 Diff
Viewer 的显示结果就认为 Patch 正确。

## 8. Command Registry

所有 UI 入口复用统一 Command Registry。

概念定义：

``` go
type CommandDefinition struct {
    ID          string
    Title       string
    Description string

    CanExecute func(CommandContext) Availability
    Execute    func(context.Context, CommandContext) error
}
```

`Availability` 应能表达可执行、禁用原因和必要的上下文缺失。

Command Context 至少包含：

-   当前 Repository / Worktree。
-   CurrentBranch。
-   SelectedBranch。
-   当前选中 Commit。
-   当前选中文件或 Hunk。
-   当前焦点区域。
-   当前工作区状态。

快捷键、Toolbar、菜单、右键菜单和 Search Everywhere 的 Actions 不得各自实现 Git
业务。

## 9. MyGo UI 架构

### 9.1 UI 模块职责

-   `shell`：窗口整体结构。
-   `layout`：IDEA 布局、Splitter、Tool Window 区域。
-   `design`：主题、颜色、间距、字体和组件密度。
-   `components`：跨功能复用的通用组件。
-   `commands`：命令展示、快捷键和入口适配。
-   `state`：窗口级 UI 状态和会话状态。
-   `features`：具体业务功能的 View 和局部状态。

### 9.2 UI 不能做的事情

-   不直接执行 Git 命令。
-   不在 View 构建过程中读取文件或访问数据库。
-   不在 View 构建过程中启动后台任务。
-   不在 Goroutine 中直接修改 UI 状态。
-   不把所有功能状态塞进一个巨型 App struct。
-   不把业务组件放进通用 components 包。
-   不让一个列表的选中状态同时代表多个业务区域。

### 9.3 MyGo 控件使用原则

-   使用 `ui.Split` / `ui.SplitVertical` 构建可调整面板。
-   大量行使用虚拟化的 `ui.List`。
-   大型树状数据优先研究 `ui.Outline`，不要直接用会构建全部节点的普通
    Tree。
-   使用 `ui.Tabs` 或自定义 TabStrip 实现 Editor 标签。
-   使用 `editor.Buffer`、`editor.State` 和 `editor.View`
    管理代码编辑器。
-   使用 `editor.DiffState` 和 `editor.DiffView` 展示差异。
-   使用 MyGo 原生 Context Menu 和输入模型。
-   使用 MyGo Theme API 和集中设计变量，不使用 CSS/Tailwind。

具体 API 必须以锁定版本的官方文档和示例为准。

### 9.4 Editor 标签状态

每个打开的文件标签应有稳定 ID 和独立状态，包括：

-   Repository / Worktree。
-   文件路径或 Diff 身份。
-   Editor Buffer / Diff State。
-   光标和选择。
-   滚动位置。
-   是否只读。
-   是否有未保存编辑。
-   Diff 的 Old / New 版本身份。

激活一个标签不能重新创建所有其他标签的
Buffer，也不能丢失用户的编辑状态。

### 9.5 选中状态隔离

至少独立维护：

``` text
CommitWorkingChangesSelection
CommitStagedChangesSelection
GitLogCommitSelection
HistoricalChangedFilesSelection
BranchSelection
ActiveEditorTab
```

不要用一个泛化的 `SelectedIndex` 代表不同面板的选择。

## 10. UI 线程与异步任务

MyGo Native UI 有明确的 UI 线程和窗口更新规则。必须遵循当前 MyGo
版本文档中的线程模型。

基本原则：

-   UI View 在 UI 线程构建。
-   后台 Goroutine 负责 Git、文件 IO、解析和计算。
-   后台任务只产生结果，不直接写 UI 状态。
-   结果通过窗口更新机制回到 UI 线程，再更新 UI Model。
-   不允许 UI 线程等待一个需要 UI 线程完成的任务，避免死锁。
-   不在每帧 View 中执行昂贵计算。

### 10.1 Task Manager

当前实现位于 `internal/runtime/tasks.go`：UI 线程独占 `Tasks`，`TaskID` 是单进程递增 uint64；`TaskInfo` 保存原始 Root、Name、QueryKey 摘要、Running/Canceling/Succeeded/Failed/Canceled/Timed out、开始/截止/结束时间、阶段和最终结果。运行态只持有请求 Context、取消入口与计时器清理入口，不保存业务快照或 UI 控件。最多 32 活动 + 100 已完成记录，32 阶段/任务及每段约 4 KiB 文本；无数据库/跨重启历史/复杂事件总线。

Bootstrap 的 `connectReadTasks` / `connectWriteTasks` 在现有 29 个异步窄端口安装完成后装饰它们。派发前在 UI 线程创建记录和有截止时间的子 Context；完成时先记录结果，再交给原回调，原指针/导航世代/根检查保持不变。即使旧回调不再安装结果，记录仍在原仓库名下结束。去重键使用操作 ID 与查询身份的 SHA-256（带字段边界），不持久保存内容/远端 URL。拒绝派发通过原错误回调交付，写回调返回输入仓库快照，避免把未执行误报为仓库刷新损坏。

任务取消只发出请求。`runningTask` 分开保存主动取消与正常完成时的计时器清理；显式取消写任务会调用派发时捕获的原 UI cancel（Init 使用自身 dialog cancel），正常 Finish 不调用原 owner cancel，避免把正常成功当成用户取消。读取若返回 nil error 而子 Context 已取消/超时，Bootstrap 向原读取回调交付 Context 错误。写结果保留真实成功/失败，不声称取消即回滚。Post-write refresh 使用独立三十秒 Context，始终读取原仓库，任务等待刷新回调才结束。退出 CancelAll 后阻止窗口关闭，待回调收齐后用户重新关闭，继续原未保存文档确认。

`application.WithTaskProgress` / `ReportTaskProgress` 是可选窄阶段报告端口；默认无操作。Update 与后台远端适配器仅报告字符串，Bootstrap 经 MyGo `RunOnMain` 更新 runtime 状态并 invalidate，不从 worker 修改 UI。RunOnMain 会等待主线程交付；UI 不同步等待这些 worker。Executor 写锁、实际超时、Context 及写前校验不由 Tasks 替代。

UI 的 `tasks.go` 只读任务快照，以原生虚拟 List 和稳定 TaskID 展示跨仓库历史；选择与编辑器/Log/分支独立。统一命令提供显示、取消指定 ID、清理完成历史；底部 Git 增加 Tasks 页签，不改变核心区域布局，Welcome 亦可展示。阶段进度为离散检查点，不伪造完成百分比；普通写入分别报告操作/刷新结果，Update 保留 Fetch/清理/整合/恢复检查点和备份身份。功能与桌面验收仍 UNVERIFIED。

### 10.2 过期结果隔离

每个仓库会话维护 `Generation` 或等价版本号。

例如：

1.  用户打开仓库 A。
2.  开始加载 A 的 Log。
3.  用户切换到仓库 B。
4.  A 的请求稍后返回。

A 的结果必须被丢弃，不得覆盖 B 的状态。

查询结果还应通过 RequestID 或 QueryKey 识别，避免旧请求覆盖新请求。

### 10.3 刷新协调

当前后台基线由 Application.BackgroundRepository、Domain.NetworkPolicy、runtime.RunRefreshLoop、Bootstrap.repositoryWatch 与 UI.backgroundState 组成。Runtime 只合并提示并产生低频调度脉冲，不依赖 MyGo 或 Git；Bootstrap 在后台安装 MyGo WatchFiles，通过 RunOnMain 交付。UI 显式生命周期入口同步活动仓库，不在 View 启动监听/查询。每个请求拥有 Context 和指针身份，导航世代、活动根、前台读写/对话框阻塞检查防止迟到结果覆盖上下文；独立 in-flight 计数保证取消后退出前仍等待回调返回。

Git.RefreshRepository 复用既有读取过程，但相同 HEAD/分支/LogSnapshot 复用已加载 History。UI 原位安装刷新，保留 repoView 与编辑器/选择/滚动；仅文件相关版本变化时重读关联可变视图，内容相同时不重置 Buffer 或光标，必要更新保留所有选择端点和滚动。后台刷新仍读取完整工作区指纹，其成本尚未测量，不引入未经证实的缓存。

网络偏好在主线程复制为不可变 NetworkPolicy。手动 RepositoryOperation 与 BackgroundRemotePolicy 共用 Executor 和原有写锁，后台 Fetch 不绕过写锁、不整合分支。CheckIncoming 读取固定 upstream 的机器格式 ls-remote，stdout 限 64 KiB；自动传输基于 remote get-url 的有效地址分类，跳过 SSH/自定义 remote.vcs，保留 SSH 配置。策略通过 Context 传入执行器的每进程 Git 参数/环境，无全局可变配置、凭据持久化或新依赖。统一任务中心已接入以上异步端口，详见 10.1；功能验收仍 UNVERIFIED。

-   文件监听事件只表示"可能发生变化"。
-   监听事件经 Debounce / Coalesce 后触发必要的 Git 查询。
-   不要每个文件事件都触发全仓库全量刷新。
-   写操作成功后按影响范围刷新。
-   任务执行期间可以显示进度，但不能让过期结果回写。
-   空闲时不做高频轮询。

## 11. 文件监听与缓存

### 11.1 文件监听

监听工作区变化时：

-   排除 `.git` 内部文件的普通递归监听。
-   排除构建目录和明显无关目录。
-   处理原子替换、重命名和连续写入。
-   监听事件必须 Debounce。
-   最终状态以 Git CLI 查询为准。
-   外部修改可能发生在任何时刻，写操作前必须重新验证。

### 11.2 缓存

先建立正确性，再建立缓存。

可以优先缓存：

-   已解析的 Commit 页面。
-   已解析的 Diff 文档。
-   文件内容快照。
-   UI 会话信息。

缓存键必须包含 Repository / Worktree 身份和对象版本。仓库切换、HEAD
变化、Index 变化和文件监听事件都应能使相关缓存失效。

第一阶段不要引入 SQLite。只有在数据量、查询模式和基准测试证明 JSON
或内存缓存不合适时，再单独提交 ADR 评估 SQLite。

## 12. 持久化

建议将用户数据保存在 MyGo 用户数据目录或平台推荐目录下。

分层：

  数据              初期方案
  ----------------- ----------------------------
  用户设置          JSON
  主题              JSON
  快捷键            JSON
  面板尺寸和布局    JSON
  最近仓库          JSON
  仓库会话          每仓库 JSON
  Editor 标签会话   JSON
  Git 状态          不作为持久化事实，实时读取
  历史查询缓存      先内存，后续按需评估

写入配置时使用临时文件 +
原子替换或等价安全方式。配置损坏时提供恢复默认值和备份机制，不应阻止应用启动。

Git 凭据不得明文写入 vela JSON 配置。优先使用 Git 自身的 Credential
Helper、SSH Agent 和系统凭据机制。

## 13. 错误处理

错误需要有机器可判断的类别，不要只依赖 stderr 字符串。

建议区分：

-   RepositoryNotFound。
-   GitNotInstalled。
-   InvalidRepository。
-   InvalidRef。
-   DirtyWorkingTree。
-   OperationInProgress。
-   MergeConflict。
-   RebaseConflict。
-   AuthenticationFailed。
-   RemoteUnavailable。
-   PatchApplyFailed。
-   OperationCanceled。
-   PermissionDenied。
-   UnknownGitError。

底层保留命令、退出码和必要输出供日志诊断；UI
展示面向用户的摘要、影响和下一步建议。

不要向 UI 暴露未经处理的原始 stderr 作为唯一错误说明。

## 14. 测试策略

日常开发按变更影响范围选择相关包、受影响的调用方和集成场景进行验证。纯文档变更仅做文档检查。仅在定向验证无法覆盖影响范围、阶段验收或发布流程明确要求，或者用户明确要求时运行全量测试或仓库级静态检查。阶段门禁仍以各 Gate 所列证据为准。

### 14.1 Domain 单元测试

测试模型、状态转换、分支 ahead/behind、操作可用性和 Diff 结构。

### 14.2 Git CLI Adapter 测试

使用临时仓库和临时配置验证命令和 Parser。

### 14.3 集成测试

通过临时 bare remote、多 clone 和多个 Worktree 测试：

-   Fetch / Pull / Push。
-   Branch tracking。
-   Merge / Rebase 冲突。
-   Cherry-pick / Revert。
-   Stage / Unstage。
-   Hunk Patch。
-   文件重命名和特殊路径。
-   操作取消与失败恢复。

禁止在真实开发仓库上运行测试用破坏性命令。

### 14.4 MyGo UI 测试

使用 MyGo Tester 验证：

-   面板显示和隐藏。
-   Splitter 尺寸变化。
-   Tabs 切换和保留状态。
-   焦点转移。
-   快捷键优先级。
-   Context Menu。
-   列表选择和多选。
-   命令禁用原因。
-   空状态、加载状态和错误状态。
-   可访问性名称和状态。

### 14.5 性能测试

建立可重复的固定数据集，测量首屏、滚动、Diff、刷新和空闲资源占用。测试报告必须包含机器、窗口尺寸、数据规模和运行步骤。

## 15. 启动与依赖组装

`main.go` 只负责启动应用，不承载业务逻辑。

Bootstrap 负责：

1.  加载配置。
2.  初始化日志。
3.  检测 Git。
4.  创建 Persistence Adapter。
5.  创建 Git CLI Adapter。
6.  创建 Task Manager、Refresh Coordinator 和 Watcher。
7.  创建 Application Use Cases。
8.  创建 UI App State 和 Native Window。
9.  注册菜单、快捷键和更新能力。
10. 处理关闭和资源释放。

应用启动失败时提供明确诊断。Git
未安装时，允许进入设置或说明页面，不应出现无意义的空白窗口。

## 16. 阶段门禁

### Gate 0：技术可行性

通过条件：

-   Go 和 MyGo 版本固定。
-   Windows 最小 Native Window 可以启动和关闭。
-   基本输入、剪贴板、文件对话框和主题验证完成。
-   Editor、DiffView、List、Split 和 Tester 的最小示例运行通过。
-   构建和测试命令记录在 README。

### Gate 1：架构骨架

通过条件：

-   依赖方向明确。
-   Bootstrap 能组装空壳应用。
-   Domain / Application / Adapter 有最小可测试边界。
-   Task Manager 和 UI 更新路径验证完成。
-   没有 UI 直接执行 Git CLI 的代码。

### Gate 2：设计系统和 IDEA 主界面

通过条件：

-   主界面区域和顺序与产品规范一致。
-   Dark / Light 主题一致。
-   所有面板能调整、隐藏和恢复。
-   Editor 标签独立。
-   Working Changes、Git Log 和历史文件选择状态隔离。
-   焦点、快捷键、右键菜单和键盘导航通过测试。
-   使用 Mock 数据完成完整视觉验收。
-   **Gate 2 未通过，不得实现真实 Git 操作。**

### Gate 3：Git 只读能力

通过条件：

-   Parser 覆盖特殊路径和大型输出。
-   Git Log、Branch、Status、Diff 有集成测试。
-   仓库切换不会被旧请求结果污染。

### Gate 4：Git 写操作

通过条件：

-   写操作串行化。
-   破坏性操作有确认。
-   过期状态会重新验证。
-   取消、错误和冲突有恢复策略。
-   所有写操作有临时仓库集成测试。

### Gate 5：发布

通过条件：

-   Windows 构建和安装验证。
-   自动更新和签名策略验证。
-   崩溃日志、诊断信息和版本信息完善。
-   关键功能回归和性能基线通过。

## 16.1 Project 文件操作边界

Project 文件管理使用 Domain 的 ProjectPathSnapshot / ProjectFileOperation，以及 Application 的窄 ProjectFileManager 端口。Git CLI Adapter 内的 ProjectFiles 只通过现有 Executor 复核仓库身份并共享写锁，实际文件操作使用 Go 标准库 os.Root；View 不读写文件。Bootstrap 在后台执行、通过 RunOnMain 返回结果，并注入锁定 MyGo v0.8.3 的 Shell.TrashItem / Shell.OpenPath 系统集成，无新依赖或持久化格式变更。

命令复用 Project、编辑器菜单和文件树快捷键；检查操作上下文、未保存文档及路径边界。确认前异步捕获源树指纹，写入前复核，切仓库/刷新/关闭取消待确认读取；写入占用现有 repositoryWrite/取消通道，结果仅安装到对应仓库。空目录通过 Git ls-files --others --directory --exclude-standard 读取，不作为普通文件送入编辑器或文本搜索。复制排他创建目标，移动先复制再回收源项，禁止覆盖目标和永久删除降级；错误保留上下文及可能的部分结果。Source Buffer 写入期间设为只读，成功移动保留编辑状态并更新路径，删除关闭本地标签，历史标签不变。

## 16.2 项目搜索与替换

Application 的 ProjectSearchQuery/ProjectSearchInput 定义字面或 Go regexp 查询、路径掩码、显式忽略文件发现和未保存文本覆盖；纯匹配生成 LF 定位指纹、原始字节指纹及受限替换预览。ProjectReplacer 是独立写端口。普通 filesystem adapter 负责 os.Root 范围内的只读发现/扫描，不引入 Git Service；Git CLI Executor 的 ReplaceProject 共享既有写锁，先预检全部目标，再复用 SaveFile 的私有持锁实现逐文件写入。Bootstrap 后台执行和 RunOnMain 回调机制不变，无新依赖或持久化格式变化。

UI 复用 Find/Replace 原生命令和虚拟列表，查询变化使预览失效；只允许完整且无读取失败的结果写入，匹配未保存 Buffer 时拒绝。写入期间目标编辑器设为只读，使用既有 repositoryWrite 取消、关闭保护、原仓库刷新及过期请求隔离。跨文件部分成功通过 ProjectReplaceResult 显式返回，失败不回滚已写文件，所有目标视图按实际磁盘身份重读并保护随后输入。

## 17. 架构决策记录（ADR）

影响长期维护的决定必须写入 `docs/adr/`，至少包含：

-   背景。
-   决策。
-   考虑过的替代方案。
-   优缺点。
-   对现有模块的影响。
-   迁移和回滚方式。

以下事项必须有 ADR：

-   更换 Git 引擎。
-   引入 SQLite。
-   引入插件系统。
-   改变 UI 框架。
-   改变 Git 写操作并发策略。
-   引入新的跨平台原生依赖。
-   改变持久化格式。

不要为普通小组件或局部实现细节创建 ADR。
