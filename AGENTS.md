# compare-figure

图形比对与独立原生Go矢量绘图阶段。生产比较/会话基线固定DocVision `20faca6aaaf8d9469b3ee68e5ff7477437b7a0d7`；当前Go runner提供luadraw/mixed/latex/asymptote四策略，默认LuaDraw3.7，支持真实宿主检测、LuaDraw两阶段回退、通用文档附图、11工具、三协议会话、复核与幂等续跑。

## 快速开始

读 [README.md](README.md)、[Go运行与导出](docs/go-native.md)、[历史迁移说明](docs/migration.md) 与 [开发记录](docs/notes.md)。正式入口为build/compare-figure（run/tools/compare/environment/build/export-skill）；Go1.25+在go/构建，缓存放build/go-cache。源码Skill需在目标机用Go1.25+首次构建，运行无需Go或Python；编译/PDF操作需要真实可用的Lua/TeX/Poppler。

Python模块和测试仅放dev/python，使用conda base（本机/home/lingnc/miniconda3/bin/conda）；共享环境只读，RE2本机依赖在忽略的.deps/，不得修改共享环境。旧compare.py及dev/python/viewer.html保留历史评分/查看用途；根viewer.html为现专用会话工具。

## 状态与范围

- 两张新LuaDraw任务图11.86/11.87使用新独立runs。现有网关禁Chat Completions、只支持Responses，因此仅新配置api改responses，model/key/endpoint不变。捕获429确切为user requests-per-minute limit exceeded，任务临时入口以至少20秒请求间隔与429退避串行运行；不将诊断/节流脚本或私有配置提交。实跑暴露模型为每个optional字段填默认，document_search同时传四flat=0和region导致连续冲突。模型Definition与fallback现只暴露canonical region；Execute/CLI仍支持旧flat，真实混用继续拒绝，不放宽来源/裁剪边界、不篡改旧转录。共享提示词同步region对象写法，修复后从原检查点恢复，不称前述错误调用已完成绘图。

- 用户新指定双环图用于README效果展示：原始位图→compare-figure/Agent工具流程箭头→最后解析裁切且保留可见红线的最佳PDF。展示不写来源书籍、图号或实验参数，不宣称一键全自动；只核准assets/showcase/two-tori.svg与two-tori.pdf作为公共首页素材，原图嵌在展示内，不额外提交原始JPG或完整run/session。右图由最终PDF保留矢量，不以PNG代替。生成入口scripts/build_showcase.py接收显式original/redrawn路径，按用户追加要求使用XeLaTeX（fontspec/Noto）排PDF、pdftocairo -svg转SVG，右图只裁空白不改路径；中间TeX/日志在build临时目录。PDF约88KB、1张原位图/2294矢量绘图对象；SVG约1.07MB、1内嵌图/2384paths、无外链/JS，字形与整图已实际看过，Chromium最终SVG1200×580截图显示正常。预览和校验放build/showcase-validation；README直接嵌SVG并提供PDF链接。不修改固定vendor、原session或重跑模型；其余生成PDF/位图继续忽略，展示素材不自动加入运行Skill分发清单。

- 最新文档源规则：用户要求补充经验可被模型document_search实际检索，并优先通过doc-library来源配置管理；经验正文docs/luadraw-surface-rendering.md，标准来源YAML在docs/document-sources/{luadraw,mixed,asymptote}.yaml。documents.New仅按mode加载通用Config，根/配置缓存相对YAML，cacheDir显式参数优先；严格拒未知字段/第二YAML文档/无效配置，只有缺文件回旧官方来源；legacy独立root不误挂祖先配置。source精确、不再暗扩luadraw，省略source=当前配置全部，sources数组可显式查两者，notes独立ID和include仅一md；doclibrary核心/固定vendor不改。此前临时写死notes源及family扩展已完全移除，后续禁止为单篇经验再写Go特判。自动/半自动/独立CLI共用此New入口，需新进程或重新初始化加载配置。README/SKILL/检索文档及mixedpolicy同步，参考与三配置均随Skill导出。documents全包20.945s/vet/build、distribution0.187s、runner提示词/高频专项0.016s及实际732文件导出SHA与包内search/read/exactsource核验通过；旧luadraw兼容包回归72.292s亦通过。实搜Dscene3d 网格 毛刺首条source=luadraw-notes，官方source=luadraw不含notes。没有付费模型或原会话改写。
- 检索输出重新设计已实现：独立doc-library commit ce677c8在用户明确授权下推送上游（本次例外覆盖该库仅本地提交约定，不授权未来自动推送/标签/Release），本项目固定remote pseudo v0.0.0-20261127125450-ce677c8064e7。核心默认v1兼容、请求output_version2/v2 wire只results正文，bbox默认关闭且true只pct；后置投影不影响render/frequency，输出配置不改变旧cache namespace/key。分页按实际候选，预算早停可next、恰好末页不虚报，snippet截断独立。
- compare-figure documents.New/NewWithOutput默认宿主v2，source YAML和docs.output配置可选version/include_bbox，NewConfig遵循独立库默认v1。runner docs.output→options.doc_output→开场frequent/HarnessDefinitions/Execute一致，CLI --output-version/--include-bbox及JSON/JSONL/工具单次参数贯通，请求显式false覆盖启动true，null/0继承启动默认。模型文字去cachehash/pointbbox/pagesize/PDFblock伪行号，保留source/doc/page或source行、频次ID，按需单套bbox_pct；重要未知warning保留、标准分页/截断尾注各一次。机器v2不重加Text/cache/provenance/document_images，app外壳仍tools/1并以output_version标文档输出；显式v1保丰富机器结构。文档/两system提示词与示例更新，不修改旧会话。
- 本轮验证：doc-library全仓test/vet、核心race、Linux构建/Windows交叉编译；官方PDF3相同命中2091→878字节（此例58%），隐藏bbox自动crop444×92保持，bboxpct裁图348×24已看文字，暖cache/history共用。host临时模块全链回归除2已修复断言及旧pin导出外均通过；正式remote pin后生产build、app完整83.438s/documents21.677s/distribution0.265s与全仓vet通过，app可移动源码Skill独立构建测试通过。无付费模型/原session/vendor改动；最终独立生产包的查询验证记录build/output-validation。

- 图5.3 Asy会话的view_pdf/compare图片实际已落盘，viewer误报未找到来自首次walkDirectory清单未含运行后新增media。现视口惰性加载会按当前session的parentHandle精确getDirectoryHandle/getFileHandle(create:false)发现缺项，不整树重扫/不轮询；每个await检查generation，切会话/根期间不污染索引/File/URL/DOM，同路径请求去重。media相对路径拒绝..、绝对路径和外部scheme，空JFIF MIME照常解码。兼容(tool_call_id=...)和旧(call id)回执归属。Chromium真实5.3会话注入只读句柄测试8/8引用图naturalWidth>0，目标1280×1175/900×826，初扫0媒体读取、未引用original0读，entries两次不增加，缺/异常/网络0；证据runs/viewer-late-media-validation，截图已查看。三locale Node及竞态/路径/按需读取回归PASS，未改真实session或调用模型。
- document_search具有search/read/view/list/find/grep/frequent；list含文件与目录(kind directory、尾/)，目录是文档source根相对路径而非宿主cwd，无cd状态，空/不允许目录不展示。Asy手册正确doc/asymptote.texi，模型误填根asymptote.texi时旧error仅5建议被顶层LICENSE占满，看不到doc/examples；改为最多32条目录优先并给具体list参数，工具doc字段说明补示例。原上游doc-library只读依赖未改。documents test20.892s/vet/build及真实list/find/read成功，docs/luadraw-doc/viewer说明同步。

- 最新图片规则：从source-md引用选中原图后，view_image默认用真实原图所在目录查同目录文件；完整Markdown本地链接用与启动选图相同的解析器建立映射，支持非images目录、URL编码和明确链接的../外部图片，其他路径仍限制在已提供根内，不请求远程图片。Markdown目录为额外根，--images-dir仅回退；自动runner和半自动tool入口均贯通，不把会话media/original复制目录误当书稿图像根。重叠搜索根去重，真实同名歧义仍提示。用户图5.3原报错是旧view_image总剥images/而images-dir指向其上级；现无需精确传images目录，保持原命令或省略images-dir均可。实际tool4次原图/邻图×完整引用/裸名成功，证据runs/image-reference-validation/result.json，无模型请求、不改原会话。
- 高频自动开场摘要默认及上限已按用户改成最多8条、每行1024字符、总4096字符，runner/documents两层硬锁同时改，显式更小预算继续有效；8条不是保证全出，先受总字符预算约束。主动document_search frequent工具默认仍256每行，可显式line_chars1024，不混称自动摘要预算。新上限/被动不计频/小预算及图片路径测试PASS，runner/documents回归与相关vet/构建成功；全drawing宿主真实TeX集成另因standalone.cls缺失失败，不能宣称全仓无条件通过。README/SKILL/配置及检索文档同步，具体本轮验证见notes。

- 用户要求配置默认位置与Agent操作明确：run/auto/tools.operation=run按显式--config/JSONconfig > COMPARE_FIGURE_CONFIG > cwd的runner.yaml/yml/json > Skill资源根同名顺序选择一份配置，不合并；--config none禁用文件发现，没文件沿用CLI/env/内置默认。显式/env和已发现文件无效立即报错，不扫描父目录/HOME、不自动猜runner.local.*，后者需明确--config/env选取。run --environment同规则，独立compile/docs/compare/tool/environment不读runner配置。run --check返回config_path仅路径，不输出内容/不触达模型也不验证API密钥。SKILL开头已加先发现再询问：先用已有配置，不重复问model/URL/key，首次先check；example仅示例、真实runner.yaml输入配置、run/result/session是状态不能混用。模型/URL/drawing/session/view/compile/figure_check/docs可从配置省略CLI，--original或source-md/image-ref及--out仍任务输入；相对路径按进程cwd。完整docs/configuration.md及README/install/example说明随源码Skill导出。禁止输出真实凭证或覆盖已有配置。

- 用户最新要求取消每次提交的自动CI，只需要推标签自动Release：ci.yml仅保留workflow_dispatch手动入口，无push/PR/workflow_call；release.yml不再调用CI或needs checks，仅v*标签触发版本一致校验、打包、checksum与创建Release。普通push/PR不运行Actions，需要完整检查时本地跑或手动触发。v0.3.0已在用户授权下正式发布，tag指向07b98b0，Release source Skill运行45571780693 success，ZIP与.sha256已上传；本次发布策略调整不重写既有标签/附件，后续tag生效。

- 最新用户要求已完成：四绘图库提示词参照完整LuaDraw规范，go/runner/prompt.go统一DrawingPrompts加载；库policy+共享drawing_figure.system完整workflow，四mode同一user骨架恢复原尺寸/看图预算/Context动态围栏/查文档到提交步骤，提醒/复核后续轮/反馈也为文件。模板路径和作用见docs/prompt-guide.md；新模板适用于新会话，续跑原system/user不改写且system只出现一次，已接受无密钥恢复不依赖新模板。一次性变量替换不重展开Context，动态围栏保护内嵌代码，四mode原尺寸Hint可保留about39%页宽（前次explicit_mm漏page_w历史不改）。用户明确允许删除unused旧latex_figure.system.md/user.md，现已删除；只保留仍用于视觉复核的latex_figurecheck两原件及SHA，docs/upstream.json保留历史来源hash并标明移除。此要求覆盖下方历史“四原模板必须本地保留”，不得自动恢复unused两文件。
- viewer已增加固定KaTeX0.18.2本地JS/CSS/60字体/MIT及逐文件SHA，支持美元/双美元/反斜线圆括号/方括号数学，配合scripts/viewer_math.js先保护公式再Markdown；代码围栏与行内源码、未配对符号不排版，坏公式保留原文，不上传、不需CDN。模板Context围栏内数学显示源码是预期；正文/展开思考与工具文字可排版。资源与说明docs/viewer-math.md/katex-upstream.json随源码Skill导出，HTML必须与相邻脚本/vendor一起保留。受控trust=false、异常捕获、公式缓存256、宽公式内滚动，原折叠/分页/媒体按需加载保留。
- 本轮验收：Go全仓test/vet/build全部PASS（runner243.777s、app79.867s、render80.307s、drawing54.107s），新模板/上下文安全/metadata/page-width/旧system只一次及accepted无密钥与复核stub回归均通过，无付费模型调用。Node实际KaTeX四分隔符/多行/代码/escapedcurrency/未配对/坏TeX/XSS/缓存、既有viewer测试在3locale均PASS。Chromium实际file://分别打开checkout与独立解压包、HTTP(S)blocked，图6.9真实70事件显示106KaTeX节点，独立4公式2display样例/代码literal/unpaired保留/本地fontsready，网络0/异常0，390px无横向溢出，screenshots已查看。证据runs/viewer-math-{validation,package-validation}，math测试直接渲染真实rows+独立fixture，不改历史会话，也不把缺注入媒体句柄称为图片已加载。
- 最终源码包本地CI步骤/全部726文件SHA/独立解压Go构建/两文档CLI/所有新prompts字节一致/旧unused移除/KaTeX及doc链接/导出后真实离线browser均PASS；build/prompt-math-validation/export ZIP SHA1db51e4dc199ee08c754622caa82bb35d64f9ea002d339b4ae75c2e3e3870ab3，未覆盖前轮dist已交付包。新增静态字体/JS为viewer所需资源，不附平台程序/开发测试/运行数据。此轮保持VERSION0.3.0、仅本地更改未推送/未tagRelease，不以早前fea71ed远端CI代替本轮结果。README/SKILL/notes/rootAGENTS同步。

- 用户新授权本地测试图6.9，使用原gpt-6.1-sol自动runner及Asymptote策略，输出runs/asymptote-figure-6.9，旧LuaDraw/原书稿不改。原参考100.48×119.58mm、17%/10%/8submit等不变；33模型轮/41工具/3轮比较，一次submit正常通过，最终重叠30.952623%、尺寸偏差0.297153%、uncertain=false/accepted_as_is=false/warnings=[]。旧LuaDraw25.010796%/1.172448%，本例提升5.941827百分点；不据单图推断普遍优劣。主线已实际看新最终预览，球面/圆锥/D2/剖面线/虚实与标签具备，PDF无bitmap记录。首次交付因Asy源码显式settings.tex=xelatex而外层auto选Lua导致xeCJK报错，原错误与会话保留；按同一accepted态--resume --engine xelatex恢复成功，api_contacted=false仅指恢复，不否认首次模型实跑。只复制submitted.tex到新目录、Xe→Asy→Xe及修复后auto模式真实重编均成功。go/render auto现尊重Asy独立语句行显式settings.tex选择，引擎显式flag优先；新增中文Asy+孤立TeX回归PASS(3.04s)、render vet/构建和真实figure6.9 auto重编PASS。此追加修复/记录仅在本地，未推送、未重新打包源码ZIP，不覆盖上一轮已成功CI的提交证据。

- 当前新需求覆盖下方历史“强制LuaDraw”政策：四绘图库策略luadraw（默认）、mixed（模型按图选LuaDraw/Asymptote/原生LaTeX）、latex（仅常规TeX绘图）、asymptote（Asy）。配置drawing.mode与CLI/JSON drawing_mode；TeX engine仍独立。编译/工具/runner/复核/接受态/可编辑源码交付与fingerprint贯通，模式变更不得静默重用旧成品，原比较/尺寸/退让回执保持。新增CTAN Asymptote3.16官方文档及.asy示例到vendor/asymptote-3.16，出处/许可/SHA见docs/asymptote-upstream.json；旧LuaDraw245文件原样不改。用户已sudo apt补齐asy依赖，宿主TeXLive3.09真实2D/静态3D/.asy与嵌入TeX编译、独立交付重编均通过；不改共享环境。用户已确认仅打包官方文档示例、不附Asy程序/运行库。0.3.0全仓Go test/vet、Linux构建、Windows交叉编译（未Windows实跑）、新三策略工具compile/compare/submit/二次无模型Deliver、旧LuaDraw回归、真实CLI预览与652文件源码包manifest/依赖/解压构建/文档筛选均通过，未调用模型。固定.render-NAME槽复用及失败清旧PDF，AsyHome/TeX缓存工作目录；core docs通过上游依赖，旧grep缺省大小写语义补齐。最终包SHA 0cf0025481114d4acba23a837efb3453fe3ddf983173788246dc16e4c8a4e4c2。推送和远端CI的实际运行编号与结果记录到build/task-state/current-requirements.md，不以本地验证替代远端成功。
- 文档工具新document_search/CLI document-search，旧luadraw_doc/luadraw-doc保留兼容；复用独立doc-library上游GitHub固定提交，不复制其核心或本地相邻replace。上游module仍doc-library时采用remote replace固定pseudo；构建可下载模块，源码包独立构建不得依赖邻目录。用户要求旧功能应为新库子集，逐项search/read/view/list/find/grep/frequent、预算分页/路径边界/PDF裁图/源码行/缓存/主动与被动频次差异验收，矩阵docs/doc-library-compatibility.md。缓存物理布局可重建不承诺旧文件兼容，不实现暂缓SyncTeX映射。

- 远端CI首轮45370931894：Go test/vet/源码ZIP导出校验及解压构建均成功，Node22/24因查看器测试假设中文/英文混合路径固定顺序而失败；本地en_US环境已精确复现。修复仅测试按路径集合验证存在性，保留最新数值轮次排序断言，CI两Node版本均覆盖C/en_US/zh_CN三环境，本地通过。已授权推送main，修复820376c对应运行45376474460已全success；不把测试错误称Go构建失败；远程origin现ai-npee/compare-figure，临时http.proxy使用已有localhost7897，不写全局配置。

- 安装入口install.md：独立流程包含Release校验/clone、Agent技能目录注册、Go/LuaTeX/Poppler依赖、Linux/Windows构建验证、默认当前Agent绘图及可选自动runner；README仅保留一句安装链接，install.md随源码Skill导出。用户已明确授权推送main（不含发布标签）；首次git push因连接GitHub过慢超时，安装文档验证提交后重试，结果以实际远端输出记录。

- README组织规则：开头清楚说明LuaDraw原创/参考图重绘、Markdown插图上下文、LaTeX数学配图、可编辑源码/PDF与质量检查；随后提供可复制给Agent的安装指令和自然语言使用示例，不写“给谁看的”。构建/参数/开发细节后置，完整原说明迁移docs/usage.md。Skill源码导出以CI验证+版本标签发布为目标，本轮仅本地配置，未推送/远端运行或创建Release；实际操作契约见docs/releasing.md，源代码分发政策不变。PDF到源码映射继续暂缓。

- 后续计划（用户明确暂缓）：PDF页/区域→TeX源文件及行范围的SyncTeX映射仅记录设计，当前没有必要实现；不重编手册、不修改vendor、不新增依赖或接口。具体候选方案见docs/notes.md；未来需PDF与映射同次编译/哈希绑定、路径可搬移、多来源范围及LuaDraw生成代码的映射精度说明。未经新的需求，不将此项作为未完成任务自动推进。

- 0.2.2完成：修复独立viewer的File旧快照、历史比较图预读、图片/会话/目录切换竞态、objectURL回收、全量DOM、适屏裁图和窄屏布局；最新100事件分页，工具/思考展开后排版，视口图像惰性加载，最新比较轮次与可用视图，未写完JSONL尾行暂缓。实际Chromium受控测试首载44→5文件、4000消息DOM168702→1485，其他会话正文0读取，重新选择看到新追加、最新round20、分页/快切/空目录/URL回收/390px无溢出均通过；真实8.8会话41工具、10/10图片滚动后显示正常，浏览器异常0。证据runs/viewer-audit及runs/viewer-validation。只读目录句柄不复制/上传，手动刷新/重新选择fresh getFile，未实现自动监听/轮询；仍遍历路径、完整读取解析所选JSONL，未验证极大转录/慢网络盘，不把CPU测试称磁盘吞吐。源码包307文件全部SHA/凭证排除及搬移构建/文档读页通过。用户另询问doc：已有search/read/view/list/find/grep/frequent，可读TeX/Lua文件行范围；尚无PDF页到源码行精确映射/SyncTeX，须grep页内独特文本定位，不宣称已精确反查。

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
