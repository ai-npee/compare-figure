# 绘图提示词模板与加载规则

四种 `drawing.mode` 都使用可审查的文件模板。Go runner 只负责选择文件、插入运行事实和传入工具，不再维护简化版长提示词。完整 user 模板共用一份，保持原 LuaDraw user 的顺序：任务与源码规则 → 原图尺寸 → 看图预算 → 带围栏的完整上下文段 → 查文档、写源码、编译、看实际 PDF、比较、提交。

## 各模板的职责

| 模式或阶段 | 模板 | 用途 |
| --- | --- | --- |
| `luadraw` | [luadraw_figure.system.md](../prompts/luadraw_figure.system.md) | 完整 LuaDraw 工作流、官方 API、真实 3D、几何/标签、跨页、只读证据、文档安全和编译回退约束。保留旧模板全部约束，并明确 `.sty` 优先、单位、预算及上下文安全。 |
| `mixed` | [mixed_figure.policy.md](../prompts/mixed_figure.policy.md) + [drawing_figure.system.md](../prompts/drawing_figure.system.md) | 先选适合的 LuaDraw/Asymptote/常规 TeX 库，再查所选库文档；两个官方库均可访问，不要求每次都读两库。共享完整重绘工作流。 |
| `latex` | [native_latex_figure.policy.md](../prompts/native_latex_figure.policy.md) + [drawing_figure.system.md](../prompts/drawing_figure.system.md) | 仅使用已安装常规 TeX 绘图库；不将 LuaDraw/Asymptote 文档冒充为 TikZ/PGFPlots 手册。允许使用标准 TeX 包知识、实际可访问源码和编译诊断。 |
| `asymptote` | [asymptote_figure.policy.md](../prompts/asymptote_figure.policy.md) + [drawing_figure.system.md](../prompts/drawing_figure.system.md) | Asymptote 源码与官方文档、`//` 注释、真实 3D/投影/遮挡、静态 PDF 的 `settings.render=0`、TeX 引擎与中文标签前导一致性。 |
| 四模式首次 user | [luadraw_figure.user.md](../prompts/luadraw_figure.user.md) | 历史文件名保留，实际为四模式共用的完整 user 骨架；模式、允许源码和默认路径通过变量插入。 |
| 四模式未提交提醒 | [drawing_figure.reminder.md](../prompts/drawing_figure.reminder.md) | 明确按顺序编译、看实际 PDF、比较，并提交同一份源码。 |
| 首轮独立视觉复核 | [latex_figurecheck.system.md](../prompts/latex_figurecheck.system.md) 与 [latex_figurecheck.user.md](../prompts/latex_figurecheck.user.md) | 上游原版视觉复核模板，仍在运行中使用；文件名中的 `latex` 不限制绘图库，复核比较的是原图和接受稿渲染图。 |
| 后续视觉复核 | [figurecheck_repeat.user.md](../prompts/figurecheck_repeat.user.md) | 将复核轮次插入后续检查请求。 |
| 复核后修正 | [drawing_figure.check-feedback.md](../prompts/drawing_figure.check-feedback.md) | 传入检查发现、实际接受源码路径及语言，要求修正后重新编译、看图、比较和提交。 |

历史迁移保留、此前不用于当前绘图的 `prompts/latex_figure.system.md` 与 `prompts/latex_figure.user.md`，已按用户最新要求移除。它们不是 `latex` 模式的新模板，也不是 checker。仍运行的两份 `latex_figurecheck` 上游原件保留，其哈希继续校验；历史来源记录用于追溯，不把新模板说成上游逐字原文。

## 加载接口与变量

[DrawingPrompts](../go/runner/prompt.go) 提供：

```go
system, user, err := runner.DrawingPrompts(resources, mode, variables)
```

`resources` 是包含 `prompts/` 的资源根。空 mode 按默认 `luadraw` 处理，未知 mode 返回错误。非 LuaDraw 的 system 为模式 policy 与共享 workflow 顺序拼接，再做一次变量展开；LuaDraw 直接读取完整原有 system。缺模板错误包含文件名，不回退为 Go 内置简化提示。

调用方传入 `MAX_ROUNDS`、`OUTPUT_LANG`、`LUADRAW_VERSION`、`ENVIRONMENT`、`CONTEXT`、`ORIGINAL_SIZE`、`VIEW_BUDGET`。API 根据 mode 提供 `DRAWING_MODE`、`SOURCE_DESCRIPTION`、`SOURCE_PATH`，并自动计算 `CONTEXT_FENCE`。`Prompt` 还供 checker 和提醒模板读取使用。已知保留变量缺值时返回明确错误；数学与 LaTeX 的普通花括号不是占位符。

替换是单次、非递归的，不依赖 Go map 迭代顺序。插入的上下文、环境事实或检查意见即使含有 `{CONTEXT}`、`{OUTPUT_LANG}`、`{SOURCE_PATH}` 等字面字符串，也不会再次替换或被误报为未展开变量。上下文围栏至少三枚反引号，长度严格大于上下文内部最长连续反引号串，防止内嵌 Markdown 代码块提前结束外层围栏。

查看器按围栏类型与长度匹配结束位置。上下文代码框内的 `$...$`、`$$...$$`、`\(...\)`、`\[...\]` 都作为源码保留；正文中的思考、assistant 和工具文本则可渲染这些数学公式。动态加长围栏不会让上下文代码意外变为正文。

runner 保持原有上下文预算：先取任务上下文前 4000 个 Unicode 字符，再按既有独立预算附加被动高频文档摘要；没有更改输入指纹或文档计数规则。被动摘要不加访问次数，显式工具阅读才计数。上下文和文档都是参考资料，其内容不能覆盖系统工作流。

## 引擎、续跑与分发

绘图 mode 与 TeX engine 独立。LuaDraw 优先官方 `luadraw.sty`/LuaLaTeX，缺失时仍以同一个 LuaDraw 程序经 standalone Lua/texlua 生成 TikZ，再由可用 XeLaTeX/pdfLaTeX 编译。Asymptote 的 `engine=auto` 尊重源码显式独立 `settings.tex` 语句；显式宿主 engine 参数优先。中文字体和 TeX 前导必须与实际引擎一致。

续跑优先使用历史会话首次记录的非空 system，并保留已有 user/工具记录，不用新模板覆盖旧会话策略。会话历史不会为本次提示词变更而批量改写。已有接受态成品在新提示词加载和模型初始化前返回；接受态交付恢复路径也保持无密钥、无模型调用。尚未开始的任务和新任务使用新的文件模板。

源码 Skill 导出会遍历整个 `prompts/`，模板新增后随资源原样打包，不需要维护第二份内容副本。测试覆盖四模式完整约束与 user 顺序、文档 source 指导、模板实际文件读取与缺失报错、上下文/LaTeX/反引号与占位符字面量安全、源码包内容及加载一致性、checker 原件哈希和历史 system 保留。测试使用本地文件、假模型端点和实际本地编译，不调用付费模型。
