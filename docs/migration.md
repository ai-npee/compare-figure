# DocVision 矢量图阶段迁移说明

基准是本机源仓库的提交 `20faca6aaaf8d9469b3ee68e5ff7477437b7a0d7`。本轮没有拉取远端，因此“最新”指该本机 HEAD，不宣称是远端当前 HEAD。源文件哈希在 [upstream.json](upstream.json)，用于固定迁移依据；运行不需要访问该源仓库。

## 当前：完整原生Go runner与Go-only Skill

当前绘图库策略见 [四模式配置](drawing-modes.md)，完整文件提示词与旧模板移除说明见 [提示词指南](prompt-guide.md)。查看器支持 [离线数学公式](viewer-math.md)。下方标注“前一阶段”的强制LuaDraw与四模板保留，是当时的迁移记录，不是当前策略或文件存在性要求。

最新用户要求已将完整Python runner及11工具再迁移为原生Go：会话/三协议与SSE、转录压缩续跑、LuaDraw运行/文档、测量/比较/提交、复核交付、幂等自动恢复和可移动多平台导出。正式入口为Go的run/tools/compare/environment/build/export-skill；Python仅保留开发原型与对照测试，不是导出选项或运行桥接。包映射、构建和JSON契约见 [go-native.md](go-native.md)，真实测试与平台边界见 [notes.md](notes.md)。下方Python表格与早期“未运行Go”文字为历史来源追溯，不表示当前Go能力边界。

## 后续 LuaDraw / Go 比较升级（前一阶段记录）

用户在生产迁移完成后，明确要求强制使用LuaDraw、升级Go并增添文档搜索附图。以下生产迁移表描述的是仍保留测试的原始基类，**当前runner默认接线**是 [LuaDrawHarness](../dev/python/luadraw_harness.py)，使用新增的LuaDraw提示词和第11个 `luadraw_doc` 工具。原四份提示词未改，不把新策略混称为原样迁移。

LuaDraw固定3.7（邻仓0.3.6是exam-zh），保留245个官方分发文件及哈希。编译回退参考本地可读 `origin/feat/skills` 的 `f0c00e975588bbea10f1f3873f2cb53f9d1829c0`，包括两阶段Lua展开、2D/3D压力测试和手册索引。环境检测必须实际编译；LuaLaTeX不可用时，同一LuaDraw程序经texlua/Lua生成TikZ后由XeLaTeX/pdfLaTeX编译。Go算法已升级为同一生产比较实现，并锁x/image0.39.0、提供JSONL常驻CLI；Python与Go都有真实回归。完整接口与宿主结果见 [tools.md](tools.md)。

旧TikZ正文片段会话的version2目录不能直接按LuaDraw续跑；当前新会话version3交付LuaDraw编辑源、生成TikZ、编译包装/PDF与actual backend。已接受稿交付时重新编译，防止视觉复核中未提交草稿混入。

下方“未运行Go”“43/49项”等记录仅描述此前原样生产迁移时点；当前升级测试结果在 [notes.md](notes.md)。

## 迁移范围

使用 `go/internal/latex/tikz.go` 的 **RunTikZSession 生产接线**作为范围，取代上一版参照 `figure_compare/runner/main.go` 实验程序的独立重写。单图阶段的输入适配为命令行/配置；图前分类、全书调度、网页进度、书稿替换属于调用方。

| 源内容 | Python 位置 | 保留的行为 |
| --- | --- | --- |
| `latex_figure.system.md`、`latex_figure.user.md` | [固定上游来源](https://github.com/LingNc/DocVision/blob/20faca6aaaf8d9469b3ee68e5ff7477437b7a0d7/go/internal/prompts/templates/latex_figure.system.md) | 迁移时逐字复制；现已按用户要求移除这两份未运行模板，保留历史哈希，运行模板见[说明](prompt-guide.md) |
| `latex_figurecheck.system.md`、`latex_figurecheck.user.md` | [复核提示词](../prompts/latex_figurecheck.system.md) | 文件逐字复制；图像顺序为原图、重绘图 |
| `CompileFigureTool` / `buildStandalone` | [vector_tools.py](../dev/python/vector_tools.py) | 正文片段，6pt边框，基础宏包、ctex、包/库/设置提升，编译日志、页数与实际尺寸 |
| `WriteWorkFileTool` / `EditWorkFileTool` | 同上 | 源文件扩展名、精确替换、追加、全替换 |
| `ReadFileTool` / `GrepTool` | 同上 | 行窗口/截断，带行号搜索，RE2语义及非法正则字面量回退 |
| `ViewPDFTool` | 同上 | 页码、裁切、局部高分辨率重渲染、毫米尺寸、软看图预算 |
| `ViewImageTool` | 同上 | 本文档图片目录、邻图文件名、裁切、放大、尺寸测量、软看图预算 |
| `ImageContextTool` | 同上 | Markdown当前/前/后图、相对行距、上下文扩展与上限、裸名歧义拒绝 |
| `compareFigureTool` | 同上 | 显式 PDF / 最新编译PDF，完整评分后裁切，通道组合、自动最差区域、zoom |
| `compareFigureImagesMM` 等 | [vector_metrics.py](../dev/python/vector_metrics.py) | 墨迹、积分网格、双起点对齐、RGB/灰度、叠图、尺寸/形状判据、原始精度分数 |
| `x/image/draw.CatmullRom` | 同上 | 按锁定的 `golang.org/x/image v0.39.0` 移植核、缩小支持域、分轴卷积及量化顺序 |
| `MeasureImage` / MinerU索引 | [vector_measure.py](../dev/python/vector_measure.py) | 祖先查找、解析根目录索引、bbox/2→pt→mm、宽度信bbox/形状信位图、裁切提示 |
| `SubmitFigureTool` | [vector_tools.py](../dev/python/vector_tools.py) | 文件/inline正文、最近编译一致性、比较前置、最好分与同次尺寸、隐藏退让参数、尝试次数、动态阈值 |
| RunTikZSession 的提醒/复核/交付 | [runner.py](../dev/python/runner.py) | 同文提交提醒、同会话修正、复核失败降级、完整文档/PDF/预览交付、不确定和merges结果 |
| 宿主Session/Client/Transcript服务 | [vector_session.py](../dev/python/vector_session.py) | 独立传输与会话适配，三协议、SSE/非流降级、批量工具回执后图片、终态停止、预算、压缩、媒体引用、续跑与接受状态收账 |

工具 schema 的名称、description、参数名和类型以生产工具定义为准。例如 `edit_file` 是 `find/replace/append/replace_all`，不是上一版的 `old/new`；`compare_figure` 使用 `zoom` 而非上一版的 `width`。`submit` 的隐藏 `accept_as_is`、`note` 不提前放进 schema。

## 刻意保留的源行为

1. `compile.merges` 是数组并累积；`submit.merges` 的定义是字符串，而源执行器没有读取它。为避免偷偷改变协议，这个不对称保留。合并结果由调用方验证并应用到书稿；单图runner不修改外部 Markdown。
2. `compare_figure` 对整图评分，然后才裁切返回证据。请求 `worst_region`/crop/zoom 仍会标记比较已完成，提交使用的仍是整图成绩。
3. 最近编译后的最好重叠分与该次的尺寸偏差绑定；不能把不同候选的好重叠和好尺寸拼接。显示值四舍五入不参与门槛。
4. 默认重叠下限0.17、尺寸偏差上限0.10、退让前置8次真实提交；退让须给说明，初始工具提示不泄露这条出口。
5. 原图毫米尺寸不明时退到墨迹宽高比，不能以不同渲染DPI下的绝对像素尺寸来判定印刷尺度。
6. live逐图阈值在每次submit重读；非零新值覆盖并保留，0不覆盖，负数关闭门槛。
7. 续跑不追加新的开场用户内容、不重复附原图，重新使用当前系统提示词；中断工具调用补回执。压缩后只按保留的历史统计。已经被接受且仍满足当前门槛的成品直接收账。
8. 可选视觉复核只补一层检查；失败、缺失结论或达到轮次上限都不会让已接受图形整张作废。
9. 保留源码中的不确定标记 `[?]`、尽力还原状态与说明，供调用方决定后续处理。

## 脱离宿主所需的适配

这些是独立运行的边界，不混称为“原文件逐字复制”：

- **提示词是逐字复制，Go执行代码是Python移植**。Python使用NumPy/Pillow/RE2、系统LaTeX/Poppler替代Go运行库。没有运行Go交叉验证，因此不宣称跨语言逐位/逐像素完全一致；解码器色彩模型、浮点求和等仍可能存在边界差异。
- 图像输入、完整Markdown、图片目录、逐图印刷尺寸及模型配置通过CLI/JSON/YAML提供，代替 `FigureEnv` 与宿主配置对象。单图输入构造只有当前图的Markdown；`--source-md`启用原有相邻图上下文。
- 逐图live阈值使用本会话 `run.json.fig_min_overlap`，代替网页修改的进度条目回调。保留状态与结果不自动删除，便于独立使用时复盘和续跑。
- 交付目标固定为输出目录中的 `submitted.body.tex`、`submitted.tex/pdf`、`result.json` 与预览；源文件名/进度数据库/跨图消费由调用方管理。已接受提交现场随转录保存，使交付前中断可以继续收尾。
- 本图工作区是实际给工具的可读写范围，原图/邻图通过专门看图工具读取；未接入DocVision整书VFS、运行网页、并发调度、计费看板、自动快照回滚和书籍后续排版阶段。
- 会话服务提供所需的协议与续跑/预算/压缩功能，但没有复制整个Go通用服务的所有厂商限流、定价、缓存探针和特定模型token估价插件。独立估算会依据实际usage校准；真实提供商仍需自己的端到端验收。

上述范围之外没有读取私人素材/密钥，没有启动替代网页服务，也没有修改源DocVision仓库。

## 验证方式

[测试入口](../dev/python/tests/test_runner.py) 对四份提示词做源哈希校验，测试媒体文件引用、工具批次消息顺序、原样续跑、门槛持久化、交付前中断恢复、压缩统计、视觉复核打回、三个本地模拟协议及SSE降级，并用真实XeLaTeX与Poppler完成正文片段→PDF→局部查看→比较→提交→交付。

[工具测试](../dev/python/tests/test_vector_tools.py) 验证工具契约、上下文、预算、尺寸、隐藏退让、动态阈值和真实编译；[算法测试](../dev/python/tests/test_vector_metrics.py) 覆盖平移、mm与aspect判据、通道、最差区域、Catmull-Rom固定像素、较大奇数尺寸的Go整数除法边界等。

本轮迁移相关43项测试全部通过（入口/会话18项、工具16项、生产算法9项），包含真实TeX/Poppler及交付前中断的无模型恢复。完整仓库回归49项中48项通过，唯一失败为旧版CLI的 `test_asymmetric_content_origin_compensation`，与迁移前一致。

未调用真实模型API，不将模拟模型会话称为实际重绘质量验收。旧版比对CLI的历史失败单独保留在 [notes.md](notes.md)，不借迁移之名更改其算法或旧分数。
