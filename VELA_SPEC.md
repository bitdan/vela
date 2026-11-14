# vela --- 产品与交互规范

> 文档状态：新项目基线\
> 版本：2.0.0-draft\
> 目标技术栈：Go + MyGo Native UI\
> 本文档定义产品行为与验收标准，不代表相关功能已经实现。

## 1. 产品定位

vela 是一款轻量、原生、跨平台的 Git 桌面客户端。

产品目标不是复制完整 IntelliJ IDEA，也不是重新实现 Git，而是：

-   以 IntelliJ IDEA 的 Git 工作流和界面布局作为主要交互参考。
-   使用原生桌面 UI，提供快速启动、低资源占用和一致的键鼠交互。
-   使用用户机器上的 Git CLI，尽量保持与 Git 命令行、IDE 和 Git
    Credential Manager 的行为一致。
-   把 Git 操作、状态、差异和工作区管理做得清晰、安全、可预测。
-   保持轻量，不引入语言服务、构建系统、调试器等完整 IDE 能力。

**产品原则：熟悉的布局、明确的操作语义、安全的 Git
行为、轻量的原生体验。**

## 2. 参考依据与优先级

发生冲突时，按以下优先级决策：

1.  用户明确确认的产品要求、布局和交互说明。
2.  用户提供的 IntelliJ IDEA 截图、录屏和实际操作结果。
3.  目标 IntelliJ IDEA 版本的实测行为。
4.  Rebased 等开源项目中可验证的交互设计，作为补充参考，而不是布局规范。
5.  Git 官方文档和 Git CLI 的实际行为。
6.  vela 当前代码和旧文档。

旧代码、旧测试和旧文档都不是正确性的证明。发现冲突时，先记录差异、核对行为，再决定是否迁移。

IDEA 主窗口布局以 [参考截图](docs/idea-example.png) 为准。

如果参考图缺失或无法辨认，Codex 必须说明缺少的证据，不得自行编造"与 IDEA
完全一致"的结论。

研究 Rebased
或其他项目时，只借鉴可观察的行为和架构思路；遵守其许可证，不复制受限制的代码、图标或设计资源。

## 3. 产品范围

### 3.1 首要能力

-   仓库打开、切换和最近仓库。
-   工作区状态、暂存区和变更文件。
-   Commit、Amend。
-   Branch、Tag、Remote。
-   Git Log、提交图和提交详情。
-   文件 Diff、Hunk Diff。
-   Fetch、Pull、Push、Update Branch。
-   Merge、Rebase、Cherry-pick、Revert、Reset。
-   Stash。
-   冲突查看与解决辅助。
-   Worktree。
-   文件历史和 Blame。
-   快捷键、命令面板、主题和会话恢复。

能力分阶段交付。产品范围不意味着第一阶段必须实现所有功能。

### 3.2 暂不追求

-   完整 IDE、代码智能、语言服务器和调试器。
-   Git 以外的 VCS。
-   内置终端模拟器（可通过系统终端打开仓库）。
-   复杂插件市场和第三方脚本执行平台。
-   在没有真实需求和性能证据前引入复杂缓存、数据库或分布式架构。

## 4. 固定主界面布局

整体布局以 IntelliJ IDEA 的 Git 工作流为准。不得因为 UI
框架控件组织方式或参考项目的默认布局而擅自改变区域关系。

``` text
┌──────────────────────────────────────────────────────────────────┐
│ Main Toolbar       Repository       Branch       Git Actions      │
├────┬────────────────────────┬────────────────────────────────────┤
│    │                        │                                    │
│ A  │ Commit Tool Window     │ Editor / Diff / Conflict Workspace │
│ c  │                        │                                    │
│ t  │ Working Changes        │ Editor Tabs                        │
│ i  │ Staged Changes         │                                    │
│ v  │ Commit Message         │ Code Editor / Diff Viewer           │
│ i  │ Commit Actions         │                                    │
│ t  │                        │                                    │
│ y  │                        │                                    │
├────┴────────────────────────┴────────────────────────────────────┤
│                         Git Tool Window                          │
│                                                                  │
│ Branches       │ Commit Log / Graph       │ Changed Files /      │
│                │                           │ Commit Details       │
│                │                           │                      │
├──────────────────────────────────────────────────────────────────┤
│ Status Bar                                                       │
└──────────────────────────────────────────────────────────────────┘
```

### 4.1 区域职责

  --------------------------------------------------------------------------------
  区域                                职责
  ----------------------------------- --------------------------------------------
  Main Toolbar                        仓库、当前分支、常用 Git 命令、全局入口

  Activity Bar                        IDEA 风格工具窗口入口

  Commit Tool Window                  当前工作区变更、暂存区、提交信息和提交动作

  Editor Workspace                    多标签编辑器、Diff、文件历史和冲突视图

  Git Tool Window                     Branches、Commit Log/Graph、Changed
                                      Files、Commit Details

  Status Bar                          当前仓库、分支、任务状态和必要的上下文信息
  --------------------------------------------------------------------------------

Git Tool Window 默认横跨主工作区底部。内部默认从左到右为
Branches、Commit Log/Graph、Changed Files + Commit Details。

允许面板隐藏、调整尺寸、最大化和恢复，但不能通过隐藏核心功能来伪装布局一致。

### 4.2 窗口尺寸适配

-   默认布局在常见桌面尺寸下完整可用。
-   窗口缩小时，优先保留主编辑区和当前操作上下文。
-   面板尺寸变化不得重置选中项、滚动位置或编辑器标签。
-   用户调整的面板尺寸应持久化。
-   View → Restore Default Layout 和操作搜索提供同一恢复命令：重置面板比例及可见性，当前工具页回到 Commit / Log；保留主题、偏好、仓库选择、列表选择、提交草稿和编辑器状态。恢复结果沿用正常退出时的布局保存机制。
-   不允许出现无法恢复的零宽面板、遮挡式布局或失去焦点的弹窗。
-   对窄窗口可以采用折叠或切换方式，但必须保持功能可达。

## 5. 核心交互规范

### 5.1 状态必须隔离

以下状态不能共用一个 selectedIndex 或一组含义模糊的字段：

-   Commit Tool Window 的 Working Changes 选择。
-   Commit Tool Window 的 Staged Changes 选择。
-   Git Log 的提交选择。
-   历史 Changed Files 的文件选择。
-   Branches 的分支选择。
-   Editor 当前激活标签。
-   Diff 左右两侧的版本和文件选择。

每个状态都应有明确的数据来源、身份键和生命周期。

### 5.2 Git Log 与 Editor 的联动

-   单击 Git Log 中的提交：更新该提交的详情、历史 Changed Files
    和提交图选中状态。
-   单击提交不得无条件覆盖右侧已经打开的 Editor 标签。
-   双击历史文件：按 IDEA 的实测行为打开或切换对应 Diff 标签。
-   已打开的其他标签必须保留。
-   切换提交时，不能丢失用户正在编辑的内容、光标、滚动位置或未保存状态。
-   当前工作区文件 Diff 与历史提交文件 Diff 必须能明确区分。

### 5.3 Branch 交互

-   单击分支：选择分支并显示上下文。
-   Checkout 必须是明确的命令，不得把普通选择隐式变成 Checkout。
-   右键菜单、快捷键和顶部菜单必须复用同一 Command。
-   当前分支、被选中的分支、目标分支是不同概念，禁止混用。
-   非当前分支的操作必须明确其目标，不得悄悄改变当前 HEAD。

### 5.4 Update 与 Pull 必须分离

`Update Branch` 和 `Pull`
是两个独立产品命令，不能互相改名、互相替代或共用模糊的执行逻辑。

-   **Update
    Branch**：以用户选中的本地分支及其追踪关系为上下文。具体行为必须对照目标
    IDEA 版本实测，记录是否先 Fetch、采用 Merge 还是
    Rebase、对非当前分支如何处理。
-   **Pull**：独立的远程拉取并集成操作。执行前明确
    remote、branch、集成策略和当前工作区状态。
-   **Fetch**：只更新远程引用，不隐式修改当前工作区或当前分支。
-   **Push**：明确推送目标和 upstream，危险推送必须单独确认。

在 Update 的目标行为未被 IDEA 实测确认前，标记为
`UNVERIFIED`，不得凭经验补全后宣称复刻完成。

### 5.5 Diff 与 Hunk 操作

-   Diff Viewer 负责展示；Patch Engine 负责生成和校验 Git Patch。
-   Stage File、Unstage File、Stage Hunk、Unstage Hunk、Discard Hunk
    是不同命令。
-   不得把 UI 的"保留/撤销差异"直接等同于 Git 的暂存/取消暂存。
-   对二进制文件、重命名、复制、文件模式变化、空文件、换行符和特殊路径必须定义行为。
-   Hunk 操作失败时保留原始工作区和索引状态，并展示可理解的错误。
-   对过期 Diff 执行写操作前必须重新验证文件和索引状态。

### 5.6 焦点、键盘和右键菜单

-   全局快捷键、面板快捷键和编辑器快捷键必须有明确优先级。
-   输入框、编辑器、列表和命令面板获得焦点时，快捷键路由必须符合预期。
-   `Esc` 关闭当前最内层弹层或退出当前模式，不得无条件关闭整个面板。
-   右键菜单必须根据被点击对象和当前选择生成。
-   列表中右键未选中行时，需按 IDEA 实测规则处理该行与现有多选状态。
-   支持键盘打开上下文菜单、上下移动、确认和取消。
-   多选应明确支持 Ctrl/Cmd、Shift、范围选择和全选。

### 5.7 Phase 1 顶栏、底栏与对象菜单

-   Main Toolbar 以 [IDEA 主窗口参考图](docs/idea-example.png) 为准，与系统窗口按钮共用一行。顶栏使用 vela 品牌图标，Windows 原生窗口使用同一图标。左侧保留主菜单、仓库和当前分支入口。右侧提供搜索和设置。工具窗口的显示控制由活动栏、Git 视图菜单及现有快捷键提供。Branches 面板的选择不改变当前分支。
-   点击顶栏主菜单在紧凑模式和横向菜单栏之间切换。紧凑模式显示仓库和分支选择器，横向菜单栏显示可直接点击的 View 和 Git 菜单。
-   Git 菜单按提交、同步、分支、日志、变更、文件和托管平台组织。Git Remotes 可在当前 Mock 仓库新增、编辑和移除远端，OK 应用本次会话的改动。GitHub、Gitee 和 GitLab 子菜单识别公共主机的 HTTPS 与 SSH 地址，提供打开仓库和复制网页地址。真实 Git 操作以及应用内 PR／MR 属于后续阶段。
-   顶栏放大镜和双击 Shift 打开 Search Everywhere，包含 All、Classes、Files、Symbols、Actions 和 Text。搜索范围为当前 Mock 项目和已打开的可编辑文件，Classes 对应 Go 类型，Symbols 查找 Go 声明。输入 `/` 或使用 `Ctrl+Shift+P` 进入操作搜索。上下键选择，Enter 打开结果或执行操作，文件结果支持 `Shift+Enter` 在右侧拆分中打开，Esc 关闭搜索。
-   Git Tool Window 包含 Log 和 Console 页签。页签下直接显示分支搜索、日志筛选及 Changed Files 工具栏。日志行从左到右显示作者、提交图、提交标题及引用、日期时间。时间显示 Today、Yesterday 或完整日期，并保留作者时间与提交时间切换。Date 筛选支持快捷范围、日历及起止时分、结束时间跟随最新提交、取消和应用。日期快捷范围基于最新 Mock 提交的时间。Log 顶部提供排序、First Parent、No Merges、刷新、Cherry-Pick、显示选项和搜索入口。Console 在此阶段展示 Mock 任务输出。Status Bar 独立显示仓库、当前分支、任务状态及当前文档上下文。
-   Log 工具栏一次只显示一个筛选面板或菜单。切换入口会关闭原弹层，已应用的条件可组合使用。
-   左侧工具窗口默认打开 Commit，通过 Activity Bar 顶部图标在 Project 文件树和 Commit 之间切换。点击当前工具窗口的图标收起，再次点击恢复原面板。Git 工具窗口开关位于 Activity Bar 底部、Status Bar 上方。Shelf 保留为 Commit 工具窗口的子页。Shelf 条目与展开后的文件各有菜单。Mock Shelf 支持重命名、移入 Recently Deleted、恢复和确认后复制到 Working Changes。若工作区或暂存区存在同路径变更则拒绝复制。Commit 工具栏使用紧凑图标，按刷新、回滚、Diff、静默搁置、视图选项、定位、展开和折叠排列。Diff 和定位使用最近操作的 Working 或 Staged 列表。视图选项提供 Directory、Repository、Module 分组和 Mock 忽略文件开关。
-   项目文件、工作区文件、暂存文件、编辑器文件、Shelf、分支、Log 提交和历史 Changed Files 按对象构建菜单。功能入口复用统一 Command。真实 Git 操作在此阶段以带禁用原因的菜单项呈现。
-   `Show Diff` 在编辑工作区显示临时预览，关闭后回到原标签。`Show Diff in a New Tab` 打开或激活对应版本的独立标签。历史菜单区分编辑本地源文件与打开只读仓库版本。
-   Commit 面板单击 Working 或 Staged 文件时复用临时 Commit Diff 标签，双击打开源文件。临时标签支持切换文件、定位差异块、并排或统一视图及固定为独立 Diff 标签。状态栏左侧展示路径面包屑，右侧展示分支及文档信息。
-   Project 工具栏提供 New、定位当前文件、展开与折叠、视图和隐藏。Scratch File 仅在当前会话中编辑，真实文件和目录创建保持禁用。
-   Project、Commit Changes、Branches 和历史 Changed Files 使用树形缩进及可单独展开的节点。Project 显示项目根和模块图标，同级目录排在文件之前。Commit 和历史变更显示分组文件数，目录分组压缩单链路径。Branches 按 Local、Remote、Tags 和斜杠前缀分层，叶节点使用短名称，菜单沿用完整引用。鼠标箭头与左右方向键控制展开状态，分支搜索使用独立的展开状态。
-   工具窗口标签与编辑器标签采用扁平样式，选中项以柔和底线标识。列表、活动栏和下拉菜单的选中项使用中性底色，保留主题正文颜色。下拉按钮的灰底仅在菜单打开期间显示。分支下拉和分支列表共用线性图标，当前分支在列表中使用星标。
-   编辑器标签菜单包括临时 Commit Diff 标签，均以被点击标签为目标，支持单个或批量关闭、固定、重新打开和拆分。批量关闭保留固定及未保存标签，直接关闭未保存标签显示禁用原因。移动到拆分区保留原编辑状态。
-   编辑器无文件标签和临时 Diff 时隐藏标签条，居中显示可点击的 Search Everywhere、Go to File、Recent Files 和 Project Navigation。对应快捷键为双击 Shift、`Ctrl+Shift+N`、`Ctrl+E` 和 `Alt+Home`。Recent Files 查找当前工作区的打开及最近关闭的源文件，Project Navigation 显示并聚焦文件树。
-   分支菜单包含与当前分支比较、与工作区比较、查看缺失提交，以及保持禁用的 Checkout / Rebase / Merge 等操作。比较和缺失提交查询使用 Mock 版本与祖先关系，不改变当前分支。
-   底部 Changed Files 提供 Diff、历史、视图选项及展开/折叠。分组、自动预览和详情显示与 Commit 面板独立。编辑器的关闭按钮位于各自标签旁。
-   Git 右下角的 Commit Details 随日志选中提交更新，显示标题、正文、可复制的短哈希、作者、邮箱与完整时间。彩色标签显示直接指向该提交的 HEAD、本地分支、远程分支和标签。Show / Hide 切换按提交图计算的包含分支列表。详情独立滚动，并保留与 Changed Files 之间的分隔条。
-   Log 父子跳转基于 Mock 图关系。有多个相邻提交时菜单列出目标，键盘快捷键使用第一条关系。无对应关系的命令显示为禁用。

### 5.8 设置

-   齿轮及 `Ctrl+Alt+S` 打开原生 Settings 对话框。Version Control 包含 Changelists、Commit、Confirmation、Directory Mappings、File Status Colors、Log、Shelf 和 Git。
-   Appearance 支持跟随系统、浅色和深色模式，以及界面缩放、字体和字号。当前界面语言为英文。Editor 可设置源文件和 Diff 的字体、字号和制表符宽度。About 显示应用图标、构建版本、架构和 Apache-2.0 许可证。
-   设置导航以整行浅底标识当前页面。一级项文字左对齐，展开箭头使用独立列，子项再缩进一层。快捷键页按场景列出 Windows 已接通的快捷键、适用范围和按键组合。
-   Version Control 一级分组默认展开，可点击标题或通过键盘展开与收起。收起保留当前页面。搜索时展开匹配的子页，清空搜索后恢复浏览时的展开状态。
-   Apply 应用草稿并保留对话框，OK 应用后关闭，Cancel 放弃自上次应用后的改动。已应用设置随应用配置在正常退出时保存，旧配置使用新增字段的默认值。
-   File Status Colors 按 IDEA 参考顺序展示文件状态，浅色默认配色以参考图为准，深色使用同语义配色。自定义颜色覆盖主题默认值，Restore Default 清除当前状态的覆盖。现有文件状态在 Changes、Changed Files 和 Project 中使用对应颜色。
-   提交消息检查、Log 显示和条数限制、文件状态颜色与 Shelf 应用策略可在 Mock 工作区中验证。Git 和 Changelist 操作偏好保存在本地，实际生效依赖后续阶段。Directory Mappings 显示 Mock 映射。

## 6. 统一 Command 规范

所有 Git 操作都必须注册为统一 Command，不能在
Toolbar、菜单、右键菜单和快捷键中分别实现业务逻辑。

每个 Command 至少定义：

-   稳定 ID。
-   显示名称与描述。
-   操作对象类型。
-   所需仓库上下文。
-   可执行条件 `CanExecute`。
-   执行逻辑 `Execute`。
-   是否需要确认。
-   是否可取消。
-   影响范围。
-   成功、失败、冲突和取消后的刷新策略。

入口包括：

-   Main Menu / Toolbar。
-   Context Menu。
-   Keyboard Shortcut。
-   Search Everywhere 的 Actions。
-   Tool Window 内的操作按钮。

同一 Command 在所有入口必须保持一致的可用性、禁用原因和执行结果。

## 7. Git 安全规范

所有写操作都必须可解释、可确认、可验证。

-   不允许静默丢弃用户工作区修改。
-   不允许静默执行 `reset --hard`、强制推送或覆盖远端历史。
-   破坏性操作必须展示目标对象、影响范围和后果。
-   执行前检查工作区、Index、HEAD、目标 ref 和 upstream。
-   检查结果可能过期，写操作执行前必须重新验证。
-   取消操作不得留下未说明的半完成状态。
-   冲突必须进入明确的冲突状态，提供继续、解决后继续或中止的路径。
-   Commit 成功但 Push 失败时，必须分别报告，不能把本地 Commit
    误报为失败。
-   所有测试使用临时仓库、临时 bare remote 和独立
    Worktree，不在真实开发仓库上测试破坏性命令。

## 8. 视觉设计方向

整体采用紧凑、克制、信息密度高的专业开发工具风格，参考 IntelliJ
IDEA，而不是通用管理后台或网页 Dashboard。

### 8.1 设计原则

-   清晰的层级、细分隔线和稳定的对齐关系。
-   主要信息优先，减少装饰性卡片和大面积留白。
-   选中、悬停、焦点、禁用、危险和成功状态必须可区分。
-   Diff 的新增、删除、修改、冲突和当前行状态必须清晰。
-   图标保持一致的尺寸、笔画和视觉重量。
-   不依赖颜色单独传达状态；必要时配合图标和文字。
-   深色主题优先完成，浅色主题必须共享同一套语义设计变量。

### 8.2 Design Tokens

建立集中管理的语义设计变量，而不是在业务组件中散落颜色和尺寸常量。

至少包括：

-   Surface：窗口、面板、浮层、选中项。
-   Text：主文本、次文本、禁用文本。
-   Border：常规边框、分隔线、焦点边框。
-   Accent：主操作和选中状态。
-   Status：success、warning、danger、info。
-   Diff：added、deleted、modified、conflict。
-   Spacing：紧凑间距、常规间距、面板间距。
-   Radius：控件、浮层和容器圆角。
-   Typography：正文、辅助文字、标题、等宽文本。
-   Density：行高、工具栏高度、列表密度。

MyGo Native UI 使用 Go 样式和主题 API，不引入 CSS、Tailwind 或 Web
设计系统作为运行时依赖。

### 8.3 主题

-   首期提供 Dark、Light 和 Follow System。
-   主题切换不能重置布局、选择和滚动状态。
-   颜色通过语义 token 引用，不在功能组件中硬编码。
-   Diff、Graph、状态栏和菜单都必须响应主题切换。

## 9. 可访问性与本地化

-   所有按钮、输入框、列表、树和菜单提供可访问名称。
-   重要状态变化能被辅助技术感知。
-   核心操作可通过键盘完成。
-   文本不得依赖固定宽度导致截断后无法访问。
-   用户可见文本集中管理，避免散落在业务逻辑中。
-   文件路径和 Git 原始输出按原样保留；展示层再做格式化。

## 10. 性能目标与验收数据集

性能必须通过固定测试仓库、固定窗口尺寸和可重复操作测量。

建议建立以下测试数据集：

-   10,000 条 Commit 的历史记录。
-   1,000 个变更文件。
-   数千个本地和远程 Branch / Tag。
-   大量 Hunk 的大型 Diff。
-   100 MB 级别的大文本文件。
-   多 Worktree 仓库。
-   中文、空格、换行和特殊字符路径。

重点观察：

-   首次启动和打开仓库耗时。
-   Git Log 首屏渲染和滚动。
-   Branch / Changed Files 切换响应。
-   Diff 首次计算和切换耗时。
-   空闲状态 CPU 占用。
-   大仓库内存增长。
-   后台任务取消和仓库切换后的旧结果隔离。

具体阈值应在基准机器上测量后确定，不得用未经测量的数字宣称性能达标。

## 11. 交付阶段

### Phase 0：架构与技术验证

-   验证 Go 与 MyGo 版本。
-   验证 Windows 原生窗口、输入、主题、字体、剪贴板和文件对话框。
-   验证 MyGo Editor、DiffView、List、Split、Tester。
-   固定依赖版本并建立最小可运行工程。

### Phase 1：设计系统与 IDEA 主界面

-   完成窗口 Shell 和全部布局区域。
-   完成 Design Tokens、Dark / Light 主题。
-   完成 Splitter、Tabs、Tool Window、Toolbar、Status Bar。
-   使用 Mock 数据呈现 Commit、Branches、Log、Changed Files。
-   完成焦点、键盘、右键菜单、面板尺寸和会话恢复。
-   完成 UI 自动化与视觉验收。

**Phase 1 未通过验收，不得开始实现真实 Git 业务功能。**

### Phase 2：Git 只读能力

进入真实仓库开发时，先用 Git → Initialize Git Repository 验证原生命令链路。用户选择已有目录及初始分支，默认使用 `main`。初始化拒绝现有或嵌套仓库，保留目录文件，成功后在状态栏反馈。命令行提供同一入口。

-   Repository、Status、Branches、Tags、Remotes。
-   Commit Log、Graph、Commit Details。
-   Changed Files、Diff、File History。
-   从仓库选择器或 Git 菜单打开本地工作树。已打开仓库可在当前会话切换。普通 UTF-8 Project 文件支持编辑和 Save / Ctrl+S；只允许保存已读取且磁盘指纹未变的可写普通文件，保留 CRLF，不自动暂存。历史和 Blame 只读。失败和并发输入保留编辑内容，重开/刷新不得覆盖未保存 Buffer；未保存内容不持久化。
-   Show Blame 读取固定 OID 的逐行作者、日期、提交和原始行号，以只读文本标签展示；工作区入口归因 HEAD 已提交版本。支持会话恢复和过期请求隔离，限制 2 MiB / 20,000 行。
-   单个真实 Project 脏标签关闭提供保存后关闭、只丢弃内存编辑后关闭、取消；保存失败、新输入或取消关闭保留标签。完成回调仅关闭原仓库原标签，不切换当前仓库。批量关闭跳过脏标签。Reload from Disk 后台重读已打开的真实 Project 文件，覆盖编辑前确认，失败、取消或读取期间新输入保留编辑，更新共享拆分视图的 Buffer 和磁盘身份。
-   Copy as Patch / Create Patch 支持跟踪工作区、暂存区或固定提交历史文件的二进制完整 Patch，最多 16 MiB / 100 个文件；拒绝未跟踪和冲突路径，不包含未保存编辑。Create 预览最多 256 KiB，经原生文件对话框将完整 Patch 保存为新的 .patch/.diff；不得覆盖已有文件、写入 Git 元数据、自动暂存或提交。外部 Patch 导入/应用另行实现。
-   Show History 使用独立原生虚拟列表，在固定起点查询 Git --follow 文件历史，每页 100 条、最多显示 100,000 条，保留各修订的真实路径/类型，支持重命名和 Git 识别的复制。列表的点击/键盘选择、Enter、右键、Diff、只读版本和复制提交 ID 复用 Command。分页失败可重试，关闭/切仓库/写入取消查询。全局 Log 不被替换，历史标签超出已加载 Log 时仍按固定对象恢复。全局 Log 分页和行范围历史另行实现。

### Phase 3：基础写操作

Commit 工具窗口支持真实整文件暂存和取消暂存。作用范围为触发操作的文件或包含该文件的当前多选集合。同一文件同时有暂存和未暂存内容时须确认。取消暂存保留工作区和 HEAD，命令完成后刷新原仓库。

Stage Hunk / Unstage Hunk / Discard Hunk 使用独立 Git Patch 预览和选择，不使用 MyGo 显示差异的位置生成写操作。适配器在写锁下重建并校验路径、HEAD、Index、磁盘版本和 hunk ordinal，再执行预检和应用；丢弃需明确永久丢弃确认。首版限于跟踪普通 UTF-8 文件的内容修改，拒绝增删、重命名、二进制、链接和 mode 变更；丢弃还拒绝 CRLF、转换属性及未保存的源文件 Buffer。失败不使用 reject/force 降级，完成后刷新原仓库及干净的关联视图，不覆盖新输入。

Clone 的目标必须为已有父目录下的新绝对目录，拒绝已存在目标和嵌套仓库，不递归子模块。失败或取消后保留可能的部分结果，明确显示目标，不自动递归清理。成功仅在仓库导航未改变且未取消时自动打开。

Commit 使用当前 Index。Amend 可修改消息或纳入暂存内容，确认时显示目标仓库、分支和提交。若本地远端跟踪分支已包含 HEAD，则拒绝 Amend。成功后刷新原仓库并清除已提交的草稿，失败时保留草稿。

-   Stage / Unstage。
-   Commit / Amend。
-   Checkout / Create Branch。
-   Fetch / Pull / Push / Update。

当前采用快进策略。Pull 显式选择远端与分支并集成到当前分支。Update Branch 按所选本地分支的 upstream 更新，非当前分支在未被工作树检出时可单独快进。Fetch 仅更新远端跟踪分支，Push 只发布显式选定的分支。与目标 IDEA 版本的 Update 行为对照仍为 UNVERIFIED。

### Phase 4：高级 Git 工作流

Merge 从 Git 菜单或分支菜单进入，显示来源和当前分支。文本冲突在 Editor 工作区显示左侧只读 Current、中间可编辑 Result 和右侧只读 Incoming。Result 以工作区已有合并内容初始化，可逐块采用任一侧或手动编辑。清除标记后应用并暂存。继续前核对未合并索引，中止前提示解决改动可能丢失。

普通 Rebase 将当前本地分支变基到明确选定的分支或标签，确认窗口提示重放提交的 ID 可能变化。暂停时复用三栏编辑器，标明 Rebased 与 Replayed Commit。Commit 区提供继续、跳过和中止，后两项需要确认。重放范围含合并提交时拒绝开始。

Cherry-pick 与 Revert 从所选 Log 提交进入，确认目标分支、完整提交 ID 与主题。合并提交必须明确选择主线父提交。每次操作单个提交，冲突复用三栏编辑器，右侧分别标记 Picked Commit 与 Reverted Parent。暂停后提供继续、跳过和中止；Git 保留暂停状态的空结果可跳过。空 Revert 未留下暂停状态时，核对并报告未生成提交、HEAD、Index 和跟踪文件未改变，保留原始诊断。跳过、中止及采用整文件一侧均须确认。外部序列仍有多个待处理提交时，继续、跳过暂不支持，保留中止及文件解决入口。

-   Merge / Rebase。
-   Cherry-pick / Revert / Reset。
-   Stash、Conflict、Worktree、Bisect。

Reset 从 Log 提交菜单或 Git → Reset HEAD 进入，确认目标仓库、当前分支和完整目标提交。默认 Mixed：移动 HEAD、重置 Index、保留工作文件；Soft 只移动 HEAD；Hard 同时替换 Index 和跟踪工作文件，必须额外勾选丢弃跟踪改动。执行前校验 checkout、Index 和磁盘原始内容指纹，保护可能被覆盖的未跟踪及忽略文件。进行中的 Merge、Rebase、Cherry-pick、Revert 或 Bisect 必须先结束。无恢复元数据的未合并索引允许 Mixed/Hard 清除，Soft 拒绝。

Stash 与 Mock Shelf 分开。保存包含暂存和未暂存改动，可明确包含未跟踪文件，保留忽略文件。恢复使用稳定提交 OID，重新校验列表版本及 reflog selector；可恢复暂存状态，始终保留原 Stash。删除单个条目必须单独确认，不提供自动 Pop 或 Clear。恢复前要求跟踪文件干净，保护未跟踪和忽略路径。冲突复用三栏编辑器和暂存入口，解决后创建普通提交；不产生虚假的 Continue/Abort Stash。要放弃应用结果，通过明确确认的 Reset 到 HEAD 清理，原 Stash 仍保留。

Worktrees 对话框显示目录、HEAD、分支及 Locked/Prunable 状态，支持在当前应用打开。新增工作树使用新的绝对目录，选择已有本地分支、Detached 提交或明确的 New Branch 模式。New Branch 显示新分支名称、起点和目标目录，拒绝已有分支，不自动配置 upstream；不隐式创建分支，不允许工作树目录嵌套。移除仅针对其他关联工作树，确认目录及分支保留；拒绝主工作树、当前工作树、锁定/缺失工作树、未跟踪或忽略文件，以及隐藏索引标记。不使用 Force，不删除分支。已打开的被移除会话保留内存标签和草稿，但禁用 Git 写入。

Bisect 第一版使用手动 `--no-checkout` 搜索。用户指定 Bad 提交及其 Good 祖先，界面显示候选 OID 并可在已加载 Log 中定位、打开只读历史版本。测试候选版本需单独进行，再明确 Mark Good、Mark Bad 或 Skip Candidate；这些操作不切换 HEAD、不改 Index 和工作文件，不自动运行测试脚本。显示唯一 First Bad 或跳过导致的不可判定结果。每次校验搜索元数据和候选身份；End Bisect 确认后清理搜索。外部 checkout 模式或自定义术语搜索的分类交由 Git，安全结束外部 checkout 搜索前要求跟踪文件干净并保护未跟踪路径。

Git → Bisect 和 Commit 面板的 Create Candidate Worktree 命令可把当前精确候选检出到单独的新绝对目录。确认框固定 Detached 和完整候选 OID，不允许改成其他引用或模式。即使候选超出已加载 Log，也可通过一致的 Bisect 元数据授权创建。步骤或搜索身份改变后拒绝旧确认。创建不切换搜索仓库、不运行测试脚本；用户通过 Worktrees 打开新目录测试，再返回原搜索仓库分类。候选工作树不会随分类、结束或新一轮搜索自动删除。

### Phase 5：完善与发布

真实仓库会话按绝对路径区分，正常退出保存草稿、选择和标签身份。启动后仅后台恢复上次活动仓库，其余保存的仓库保留为最近入口；标签按磁盘、Index 或历史版本重读，失败身份保留可重试。用户切换仓库、选择或关闭标签优先于异步恢复回调，旧结果不得抢占焦点。

Manage Remotes 的真实新增、fetch URL 修改和删除按单个操作确认并执行，保留独立 push URL；继承配置、多 URL 编辑及自定义 fetch 映射删除拒绝在应用内执行。删除复核所有远端/分支配置的 scope、origin、内容和目标远端跟踪引用，保留本地分支及服务端仓库。本地分支重命名不强制覆盖；删除仅针对完全合入 HEAD 且未被工作树检出的非当前本地分支。首版 Tag 创建为固定目标提交的本地轻量标签，不覆盖、不签名、不推送。

Commit 消息格式检查适用于真实 Commit/Amend；TODO 检查只检查新增的暂存行。失败时保留草稿和 Index，不创建提交。整文件暂存/取消暂存及 Commit 的 Context 可通过底栏 Cancel Task 或统一 Actions 命令取消，返回并刷新前保持写入忙碌状态，不宣称取消等于回滚。

-   快捷键管理。
-   首版 Shortcuts 设置支持 13 项既有工作区命令的单键组合覆盖、禁用和恢复默认，以稳定命令 ID 保存；缺省沿用原绑定，空字符串表示禁用。Assign、Reset 和 Apply 校验有效绑定的冲突、格式及锁定 MyGo 编辑器/面板/系统保留键。设置草稿与已应用映射隔离，Cancel 不改变生效状态。分发、菜单和编辑器提示复用有效绑定；原生焦点优先级、Double Shift、局部编辑器/面板键位维持现状。多段组合键和局部键位编辑另行实现。
-   多仓库会话。
-   性能、可访问性和跨平台验证。
-   自动更新、安装包和 Release。

## 12. 功能验收证据

日常变更按影响范围验证，纯文档变更检查文本和引用。阶段验收按对应功能和门禁收集证据，并记录实际验证范围与未验证项。全量测试的触发条件见 [技术架构的测试策略](ARCHITECTURE.md#14-测试策略)。

一个功能存在不等于交互正确。每个重要功能都必须记录：

  项目       必需证据
  ---------- ------------------------------------------
  参考行为   IDEA 版本、截图/录屏或实测步骤
  目标行为   vela 的明确行为说明
  状态矩阵   正常、禁用、异常、冲突、取消等场景
  实现位置   UI、Use Case、Domain、Adapter 的具体文件
  自动测试   单元、集成或 UI 测试
  人工复核   对照截图和实际键鼠行为
  结果       PASS / PARTIAL / FAIL / UNVERIFIED

禁止用"已实现""已完成"替代验收证据。
