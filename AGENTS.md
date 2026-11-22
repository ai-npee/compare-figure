# compare-figure

图形比对与独立原生Go LuaDraw矢量图阶段。生产比较/会话基线固定DocVision `20faca6aaaf8d9469b3ee68e5ff7477437b7a0d7`；当前完整Go runner强制LuaDraw3.7，提供真实宿主检测、两阶段回退、文档附图、11工具、三协议会话、复核与幂等续跑。

## 快速开始

读 [README.md](README.md)、[Go运行与导出](docs/go-native.md)、[历史迁移说明](docs/migration.md) 与 [开发记录](docs/notes.md)。正式入口为build/compare-figure（run/tools/compare/environment/build/export-skill）；Go1.25+在go/构建，缓存放build/go-cache。安装包无需Go或Python，但编译/PDF操作需要真实可用的Lua/TeX/Poppler。

Python模块和测试仅放dev/python，使用conda base（本机/home/lingnc/miniconda3/bin/conda）；共享环境只读，RE2本机依赖在忽略的.deps/，不得修改共享环境。旧compare.py及viewer.html保留历史评分/查看用途。

## 状态与范围

- 仓库整理：根SKILL.md带name/description/license/metadata.version frontmatter，可作为源码Skill直接使用；go/distribution读取该唯一说明源，导出仅更新版本元数据，不维护Go内嵌副本。根VERSION当前0.1.0；scripts/export_skill.sh从任意cwd构建指定平台并输出带版本目录/ZIP/SHA256，--skip-build显式复用指定平台；VERSION/manifest.version/Skill metadata.version一致。Python模块、algorithms、历史viewer、requirements及tests统一在dev/python；脱敏配置在examples/runner.json。根目录不再放Python，缓存统一build/（旧缓存仅归档，不删除数据）。
- 最新修复：Python历史小图平移失败已解决，levels_for不再以32格覆盖小于32px的画布，避免零面积格/NaN评分。原(1,8)平移和100%/逐像素对齐已锁定；相同裁剪质心应去重为1个起点，新增不同起点保留与24-96px网格边界测试。完整Python unittest79项：78通过、1宿主LuaLaTeX直编跳过，0失败；生产Go算法/回执和Go-only版本不变。下方77项旧结果仅记录整理时点。
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
- 本机LuaLaTeX命令存在但fontloader缓存不可写，探针如实标记不可用，已实测Lua/texlua→TikZ→XeLaTeX及pdfLaTeX。未调用真实模型，不宣称实际重绘质量、所有图像逐位一致或交叉编译等同目标机实测。
- 旧compare.py小图平移与旧校准边界保留在开发记录，不静默改历史分数。algorithms/为研发指标；viewer.html没有会话网页。独立runner不启动全书工作流、不修改外部书稿，merges由调用方消费。

## 维护约定

维持简约结构，不建立重复Skill/算法副本。重要改动同步README、本文件和开发记录。产物在runs/、预览在约定预览目录，build/和dist/忽略；不提交密钥、本地配置或私人素材。Skill导出校验Go构建信息/真实目标平台并附SHA清单，不打包Python、开发测试或运行数据。跨平台验证须明确编译与实际执行边界。

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
