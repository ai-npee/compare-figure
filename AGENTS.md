# compare-figure

图形比对与独立原生Go LuaDraw矢量图阶段。生产比较/会话基线固定DocVision `20faca6aaaf8d9469b3ee68e5ff7477437b7a0d7`；当前完整Go runner强制LuaDraw3.7，提供真实宿主检测、两阶段回退、文档附图、11工具、三协议会话、复核与幂等续跑。

## 快速开始

读 [README.md](README.md)、[Go运行与导出](docs/go-native.md)、[历史迁移说明](docs/migration.md) 与 [开发记录](docs/notes.md)。正式入口为build/compare-figure（run/tools/compare/environment/build/export-skill）；Go1.25+在go/构建，缓存放build/go-cache。源码Skill需在目标机用Go1.25+首次构建，运行无需Go或Python；编译/PDF操作需要真实可用的Lua/TeX/Poppler。

Python模块和测试仅放dev/python，使用conda base（本机/home/lingnc/miniconda3/bin/conda）；共享环境只读，RE2本机依赖在忽略的.deps/，不得修改共享环境。旧compare.py及dev/python/viewer.html保留历史评分/查看用途；根viewer.html为现专用会话工具。

## 状态与范围

- 0.2.1追加：当前Agent可无参考图创作LuaDraw图，先frequent/search/定位精读，再编写源码、compile/看PDF并交付；没有参考图不调用compare/submit，不声称比对分数。独立luadraw-doc CLI接受参数、--request/--stdin/--jsonl的裸工具JSON或docs envelope，供其他Skill/外部程序调用；不需model/key/reference/Markdown。复用现有索引与频次缓存，--cache-dir/JSON cache_dir或环境变量可设。原docs/luadraw-doc-search/tools接口兼容；专用docs入口禁止JSON切换到run/compile。文档初始化需Poppler，绘图编译才需Lua/TeX。独立调用契约docs/luadraw-doc.md与examples/luadraw-doc.json.example随源码Skill打包；公开Go luadraw.NewDocs/Definition/Execute/FrequentSnippet接口保留。Go全模块test/vet与专项回归通过；从无参考图/无密钥的另一目录实际验证CLI字段、JSON文件/裸参数、JSONL坏行继续、非docs请求拒绝、指定缓存和索引mtime复用；源码包307文件全部SHA核验通过，解压后构建并执行search/frequent JSONL成功。未重跑已完成四图付费模型。

- 当前0.2.0用户要求：仅分发生产Go源码、go.mod/go.sum、SKILL/VERSION、固定LuaDraw资源、提示词、Lua helper、注释examples/runner.yaml.example、专用只读viewer.html及来源/许可；不附任何平台二进制、开发测试、真实配置或缓存。官方Go下载go.dev/dl，目标机Agent首次go -C go build；原0.1.2单Linux包策略由本次直接用户要求覆盖，不得恢复旧默认。导出manifest schema2 source-only/go/1.25+，所有文件SHA；目标平台按本机Go确定。
- 用户全部需求与验收证据持续保存在忽略的build/task-state/current-requirements.md：压缩上下文后优先读取，不重复启动已完成真实模型任务。配置改带注释YAML .example，保留用户128轮/131072输出/256000上下文/300dpi等参数；真实runner.local.yaml仅忽略且0600，凭证不得输出/提交/打包。
- gpt-6.1-sol真实四图自动运行已完成，图6.9/8.8/8.9/8.10重叠25.01/62.33/50.89/35.19%，尺寸偏差1.17/0.83/1.52/0.58%；均正常一次submit，无best-effort及warnings，6.9及8.10保留uncertain。四份预览已查看；8.10中部K1原扫描疑点按原样保留。仅本地runs/automatic-four-figures留存；image_context与启动选图均支持[url](images/...)普通图片链接，用原书图6.9精确up6/down6验证成功。
- 文档工具同一luadraw_doc提供search/read/view/list/find/grep/frequent；搜索默认纯文字+doc/page/bbox定位，显式view整页/百分比区域才渲染，read兼容PDF doc或query+page。完整段落去重/同页几何相邻合并，snippet_chars/max_chars可调；固定库根只读，不开放宿主bash、不逃逸符号链接。真实不存在文件给路径建议，不伪造别名。
- 高频缓存使用共享build/doc-cache，按资源指纹失效、稳定段落ID及次数，主动模型frequent选择排名/entry_id/原文阅读贡献次数，仅对实际输出项计数；开场user自动附加最多8行/2048字符摘要不加次数，可在docs.frequent配置关闭。动态频次将知识段落推到前面；DocVision未来每书top10贡献全局冷启动仅记录批注，当前只维护全局缓存。
- 根viewer.html复用用户指定Test/MinerU_Tools即DocVision的专用会话UI，不使用历史Python metrics viewer；只读选择文件夹、扫描路径后按需读取选中会话/引用媒体，支持Go JSONL/run/result/比较与最终预览，不上传、不截断。Node功能测试及真实Chromium加载图8.8的41工具/10图片已验证，真实系统folderpicker由测试注入句柄，未声称自动化系统目录对话框。
- 最终submitted.tex使用内嵌官方luadraw环境，自包含可编辑源码，额外保留accepted Lua；从该TeX重编PDF并保留同一接受态指标/uncertain，缺LuaLaTeX仍走独立Lua/texlua→TikZ→Xe/PDF。四图均已无模型重交付；另将图6.9 TeX单独复制到空输入目录，LuaLaTeX直编与独立Lua→TikZ→XeLaTeX均真实成功；原指标/不确定标记保持。最终冻结版本Go全模块test/vet通过，Linux构建与Windows交叉编译通过（Windows未执行）；源码包304文件、固定vendor245文件哈希核验通过，独立解压后构建并读取官方PDF成功。Python开发对照75项通过，补齐PYTHONPATH后遗漏模块4项通过，共79项；最终文档工具真实payload、频次并发/race、查看器真实浏览器及独立TeX两路线均验收。
- pi-go仅做公开库/API/许可静态评估，不直接改go.mod；可考虑替换通用模型循环，必须保留三协议思考回放、媒体、checkpoint/中断恢复与接受态无密钥交付。评估报告另存docs/pi-go-evaluation.md。

- 0.1.1继承修复：LuaLaTeX已实测可用；旧探针/编译将TEXMFVAR/TEXMFCACHE分离导致fontloader误判，Go与Python现统一为同一自有可写根，不改HOME。回退包装补齐LuaDraw.sty同套TikZ运行库（含arrows.meta/patterns/plotmarks/decorations），用户复杂3D Stealth箭头真实编译已通过。
- 复杂测试包以用户直接上传附件为准：testdata/luadraw/luadraw-test-2d.lua（2225B，93706afa…e4dab）、3d.lua同前缀（2124B，af411326…58547）原字节，完整SHA/来源/覆盖见manifest。每份均测试LuaLaTeX和独立Lua回退四条路线，真实PDF/PNG已核验，3D生成3502行TikZ；不使用skills分支简单片段。测试区不打进运行Skill。
- 上轮验证：Python完整79项全通过、0跳过；补运行库后runtime9项再通过；Go回归与vet通过。已重建五平台v0.1.1包，只Linux amd64实跑；最新证据runs/luadraw-complex-validation。旧71/77/79项中宿主跳过的描述仅历史时点，不代表当前LuaLaTeX状态。

- 仓库整理：根SKILL.md frontmatter为唯一说明源；VERSION/Skill metadata/manifest均0.2.0，脚本默认读取VERSION，源码导出只构建临时本机导出工具本身不入包，移除--skip-build/--platforms二进制导出选项；开发build命令可显式交叉构建。Python只在dev/python，注释配置示例examples/runner.yaml.example；旧真实JSON已本地归档不提交。缓存/数据/成品在build/runs/dist忽略目录。
- 最新修复：Python历史小图平移失败已解决，levels_for不再以32格覆盖小于32px的画布，避免零面积格/NaN评分。原(1,8)平移和100%/逐像素对齐已锁定；相同裁剪质心应去重为1个起点，新增不同起点保留与24-96px网格边界测试。修复当时Python unittest79项：78通过、1直编跳过（现已消除），0失败；该项修复未改生产Go比较算法/回执。下方77项旧结果仅记录整理时点。
- 整理验证：Go完整test/vet通过；迁移当时Python unittest77项：75通过、1当时小图平移失败（现已修复）、1宿主LuaLaTeX跳过。按主题分批本地Git提交，不推送。资源原样哈希固定，upstream vendor尾随空白不做格式化。

- 最新边界：本程序是独立绘图/比较工具，不读取MinerU索引/content_list/layout/bbox，不要求DocVision工程或原PDF。正式Go已删除这类尺寸回退；历史Python原型/迁移记录只作旧版本追溯。尺寸仅explicit mm > 指定/图片旁sizes.csv（size.csv兼容）> aspect_only；CSV width/height分别保留，scale_px_per_pt只作来源、不重复缩放。返回完整尺寸文字回执和reference/candidate测量来源、页宽与normalization。
- 回执严格采用原版compare_figure Soft-ink/Ink size/alignment/crop/overlay模板，Agent工具文字+图片，不自增画布/来源/normalization说明或JSON包；测量来源仅程序JSON字段。原版compare只报告；submit在MaxScaleDev配置正数时原本就会拦截超限（上游tools.go253-355已核对），本程序不额外增加尺寸强制规则，保留原SUBMITTED/REJECTED/8次best-effort。黄金测试逐字匹配用户14.74%/1.7%回执。
- 常见Book.md+images/Book/figure.jpg+sizes.csv独立目录可用run/tool/compare --source-md --image-ref；解析本地相对/URL编码链接，目标前后20行初始上下文，完整MD供image_context扩展/邻图。多图明确选择，不改外部书稿。

- 最新用户要求：Skill明确分自动/半自动两模式，默认均17%重叠。自动run/auto使用用户model/baseURL/key，新增临时api-key与key-file接入且不持久化；半自动当前Agent自主绘制，不需额外模型或key。docs/luadraw-doc-search、compile/luadraw-compile、compare为直接CLI；tool/agent-tool提供11工具与持久化接受态，保持LuaDraw/尺寸检查与同一生产评分。
- compare/docs/compile默认按当前工作目录+操作复用固定系统临时槽（COMPARE_FIGURE_OUTPUT_ROOT可覆写），不再逐次MkdirTemp保存输出。对比图/metrics/result固定覆盖，文档预览固定预览/document.png并删除本次唯一中间图；需要保留或并行任务显式--out。自动/Agent工具工作区保留有意源码/状态，不自动清扫用户目录。compare成功计算但pass=false仍exit0，必须读取pass/thresholds；CLI threshold百分比17，JSON pass_overlap比例0.17。

- 本轮双模式验收：Go全模块test/vet通过；新增固定输出/文档重复图/无模型编译、临时key不落盘、持久11工具与重复deliver、百分比参数覆盖等真实回归。Python仅开发对照的GoCLI/GoRunner5项通过。五平台程序已重建，实际导出261文件且重复0写入，ZIP逐文件SHA与双模式清单验收；仅Linux amd64实跑，未调用真实模型。最新证据在忽略的runs/modes-validation，旧runs/go-native-validation是前版记录。
- 用户要求完整原生Go移植，Skill只提供Go运行版本；不增加Python导出选项，不通过Python子进程或Python SDK桥接任何正式能力。保留Python原型便于开发与交叉验证。
- go/session迁移Chat Completions/Responses/Anthropic、SSE/非流降级、思考/签名/加密回放、图片请求体、API重试与可取消退避、工具批次、预算/压缩、文件媒体转录/检查点和中断工具修复。Go通用厂商计费/全书服务不在范围内。
- go/luadraw迁移真实环境探针、LuaDraw编译和官方文档索引/搜索/阅读/局部PDF图；只用Go与系统Lua/TeX/Poppler，不依赖PyMuPDF。候选组合逐一实测，缓存包含宿主/资源指纹，不改HOME。
- go/drawing实现11工具与独立图片/CSV测量；RE2 grep、文件边界、上下文/邻图、软看图预算、PDF局部高分辨率重渲染、比较/提交、已接受快照/交付。Rasterize支持pdftoppm与pdftocairo，保留原图复制前测量信息。
- go/runner负责JSON/YAML配置和extends、输入/指纹、动态门槛、复核、恢复与交付；go/app为原生命令/JSONL服务；go/distribution为可移动资源和Go-only Skill导出；root Go包的ProcessRequest为共享比较API，不重复算法。
- 相同输入自动续跑；不同输入不混用已有目录。已合格成品直接返回，不需模型密钥/再次调用模型。接受后交付中断可无模型重编，初始化失败重试保留原始任务/图片。版本3 Python转录/元数据可由Go恢复，已接受产物路径可重定位。
- 默认重叠0.17/尺寸偏差0.10/8次真实submit后隐藏退让；原始精度、最好分与同次尺寸、merges与uncertain保留。显式CLI零门槛允许关闭；live逐图阈值0不覆盖、负数关闭。裁切证据前先整图评分。
- 编译主稿优先figure.lua，也接受单个真实LuaDraw环境的TeX；不能提交手写TikZ。交付重编已接受稿，使用同次分数；视觉复核可多轮工具修正，失败/缺结论/上限则警告交付已接受版本。
- 原四份上游提示词逐字保留，哈希固定docs/upstream.json；实际绘图使用新增LuaDraw system/user模板。不得静默精简或把LuaDraw扩展描述成逐字上游提示词。
- 固定LuaDraw3.7的245个分发文件、官方手册、LPPL来源及SHA锁；相邻项目0.3.6是exam-zh。回退参考本地origin/feat/skills f0c00e9，不修改外部源仓库。
- 当前Go全模块test/vet已通过，真实集成涵盖2D/中文数学3D/fallback、裁图/比较/提交、七轮复核修正、无结论保留、无密钥恢复及可移动/幂等导出。Python开发对照71项：70通过、1真实LuaLaTeX宿主原因跳过，含Python接受态由Go无模型恢复。最终构建/清单结果见开发记录。
- 本机LuaLaTeX直编与Lua/texlua→TikZ→XeLaTeX/pdfLaTeX回退均已实测可用；旧fontloader误判由缓存配置修复。前阶段未调用真实模型；当前四图已真实运行并按上方指标记录，不宣称所有图像逐位一致或交叉编译等同目标机实测。
- 旧compare.py小图平移与旧校准边界保留在开发记录，不静默改历史分数。dev/python/algorithms为研发指标；根viewer.html现提供会话网页。独立runner不启动全书工作流、不修改外部书稿，merges由调用方消费。

## 维护约定

维持简约结构，不建立重复Skill/算法副本。重要改动同步README、本文件和开发记录。产物在runs/、预览在约定预览目录，build/和dist/忽略；不提交密钥、本地配置或私人素材。Skill导出仅生产Go源码/固定资源并附SHA清单，不附二进制，不打包Python、开发测试或运行数据。跨平台验证须明确编译与实际执行边界。

---

## Agent 自举说明

本文档作为项目全局状态机与综述，能够防止代码库膨胀后上下文丢失，确保后续 AI 辅助开发时拥有完整的记忆和设计初衷。

### 实时同步要求
1. 当找到关键因素和进行关闭编程时，必须如实记录。
2. 当完成一个任务时，必须更新进度清单。
3. 当添加新的技术规范时，必须补充。
4. 当功能有较大变更时，必须同步更新 README.md。

### 维护责任
- 每次会话开始前，AI 工具应优先读取 AGENTS.md 了解项目状态。
- 每次重要变更后，AI 应及时更新此文档。
- 保持 AGENTS.md 与其余部分的描述一致性。

### 必须做的事
- 完成用户的任务后，必须回复："我已严格遵守AGENTS.md。"。
