# 开发记录

## 独立安装流程与推送

用户要求用install.md集中安装流程，避免冗长安装提示词。新增根install.md，说明源码包/仓库获取、SHA校验、技能目录注册、依赖与Linux/Windows构建、无模型文档验证及可选自动runner；README缩为一句安装链接。install.md纳入Skill导出清单，所引用相对路径在包内可用，完整仓库说明使用远端链接。用户已授权推送main；首次git push连接GitHub超过60秒过慢失败，HTTPS直连检查无响应；使用现有本地代理后GitHub可达，并确认旧LingNc地址301跳转到ai-npee/compare-figure，安装/克隆链接改为实际地址。安装说明相对链接与包内字节一致、308文件完整CI导出/哈希/解压构建/资源检索及app/distribution回归通过，证据build/install-ci-validation/export-step.log。不创建Release标签。

## README入口整理与源码Skill持续发布

按用户要求首页先解释主要功能：参考图片重绘、文字描述原创LuaDraw图、Markdown插图上下文和LaTeX数学配图，随后提供可复制给Agent的安装话术、自然语言使用示例和两种模式；构建/配置/开发放后面，不标注面向谁阅读。原有完整CLI、尺寸、会话、JSON、导出与验证说明迁入docs/usage.md并调整相对链接，详细接口继续链接既有文档，避免首页重复堆砌历史数据。Markdown/LaTeX指绘图上下文和嵌入目标，不夸大为整篇自动转换。

源码导出接入GitHub Actions：普通更新/PR测试和校验，版本标签与VERSION一致才可发布ZIP/SHA；CI构建的本机导出工具不进分发包，维持source-only，真实配置/缓存/转录/私有数据不入包。仅在本地准备工作流及发布说明，未经明确指令不推送、不创建远端标签或Release；本地验证与远端执行结果分开记录。本地验证通过：README/详细文档相对链接与首页信息顺序、两个workflow的YAML解析、所有嵌入Bash与Node语法、Go全模块test/vet、viewer和导出参数测试；实际307文件源码ZIP校验和、解压manifest逐文件哈希、版本一致性、排除规则、独立构建及LuaDraw资源list验证通过。此外主线直接从最终YAML读取并执行完整“Export and validate source Skill”步骤，307文件解压校验、VERSION/SKILL/manifest一致、包内构建和资源检索全部通过，持久日志build/ci-validation/export-step.log。远端Actions/Ubuntu安装步骤尚未执行，没有推送、tag或Release。该次仅说明与CI配置，运行功能版本保持0.2.2，PDF源码精确映射继续暂缓。

## 后续计划：PDF页面／区域到TeX源码映射（暂缓）

用户已确认先加入计划，目前没有必要实现。当前继续使用PDF文字/图片查看、grep定位和源码按行read，不新增运行依赖、不重编官方手册、不改固定vendor或现有工具接口。

未来确有需求时评估：构建阶段开启SyncTeX，生成与同次PDF严格匹配的映射；保留上游PDF原样，先以单独映射版手册验证分页。导出知识库相对路径，绑定PDF/源码SHA与映射算法版本；索引按页或bbox返回多个源码文件/行范围，复用现有read和指纹缓存。普通正文定位与LuaDraw环境/生成TikZ定位应区分粒度，不承诺图形元素能精确反查原Lua语句。文本对齐仅作为带置信度的候选定位，不能替代精确映射。目标机消费预生成映射，无需为了文档查询重新编译手册。该项当前未实现，也不是当前版本的交付门禁。

## 查看器目录实时读取与性能审查（当前，0.2.2）

用户要求检查独立viewer是否按目录实时读取、性能与页面不完善之处。它通过浏览器showDirectoryPicker的read权限取得目录句柄，扫描路径而不上传/复制目录正文；旧lazyFile缓存了File快照，同会话重新选择会停留在旧转录。另复现预读全部历史比较图、比较轮次取最早、object URL不回收、全部消息及折叠正文一次渲染，以及适屏缩放object-fit:none造成图片只显示中间一块、窄屏目录列表过长。

Chromium基线用真实8.8转录和受控内存目录句柄：加入20轮比较记录时读取44个文件；100个会话中选一份约2.99MB/4000条消息的首载约1.24s、168702个DOM节点，其他会话正文读取0；追加模拟消息后重新选择未重读文件。这是CPU/DOM和读取次数测量，不是磁盘吞吐、不声称实测系统目录对话框。证据runs/viewer-audit。修复完成并验收：fresh读取当前会话/元数据/所选比较图，最新数值轮次优先；Map路径定位、扫描与会话/图片异步取消，空目录清理旧统计；懒加载视口缩略图和比较当前图、URL按会话回收，切换/分页显式disconnect旧IntersectionObserver并清理旧思考正文映射；工具和思考正文展开后排版，时间线最新100条并可载入更早；修正preview选项、原图完整fit（支持低于10%缩放）、预览/比较图例与窄屏目录/选择器。JSONL末尾尚未完成且无换行的坏JSON暂缓，完整坏行仍显示解析错误。

最终相同Chromium受控测试：20轮比较首载44→5文件读取；4000消息首屏4002→100事件、168702→1485 DOM节点，包含固定100ms稳定等待的测量1.24s→0.16s，仅作同机CPU/DOM对比；其他会话正文读取0。重新选择读到追加消息，预览选项可用，比较只读round20不读round1，工具从10字符占位展开为1130字符正文，分页200条、快切竞态、取消旧目录扫描和空目录清理均通过，5个object URL最终全部回收，390px视口无横向溢出，浏览器异常0。真实8.8目录数据验证首屏5文件，滚动后10/10张引用图正常、41工具完整。Node parser/fresh/partial/timeline测试通过，最新报告runs/viewer-audit/after-report.json、真实会话runs/viewer-validation/browser-result.json。

实现仍是手动刷新/按需重读而非系统文件监听，未加入定时轮询；扫描O(目录条目)，所选JSONL完整读取/解析O(转录大小)，分页限制初始DOM而非输入字节。不断点载入100MB以上会话、巨大目录或缓慢网络盘的吞吐/峰值内存未作承诺；图片随所查看页逐步增加，切换/刷新时释放。

## 独立文档索引接口与原创LuaDraw绘制（前阶段，0.2.1）

用户要求让只想创作LuaDraw图的Agent/其他Skill使用既有知识库，而无需参考图。新增独立luadraw-doc入口，支持CLI字段和互斥的--request/--stdin/--jsonl；JSON接受裸工具参数或docs envelope，显式JSON覆盖CLI默认值，输入缺省operation为docs，拒绝切换run/compile。JSONL共用service与Docs实例，坏行独立报错后继续，保留request id，有失败最终exit1；单次失败exit2。原docs/luadraw-doc-search/tools调用不变。

索引/频次缓存保持原机制，CLI --cache-dir、JSON cache_dir、环境变量可设；外部程序不需模型key、reference或Markdown。文档初始化当前需要Poppler，绘图编译才需要Lua/TeX。原创图Skill流程为先frequent/精读，再编写LuaDraw、compile/看PDF、交付源码/PDF，没有参考图不报告compare分数或进入submit接受态。公开Go NewDocs/Definition/Execute/FrequentSnippet接口在独立说明中提供调用方式。

新docs/luadraw-doc.md与examples/luadraw-doc.json.example纳入源码导出，版本0.2.1；代码专项测试与Go全模块test/vet已通过。真实外部目录调用、源码包解压构建和缓存复用检验均通过：无reference/key时CLI/PDF读页、裸JSON、文件envelope、JSONL坏行和非法operation后继续有效请求，指定cache落盘且warm index mtime不变。0.2.1源码包307文件全部哈希校验、实际私有密钥/端点字符串扫描无泄漏，解压独立构建后search/frequent JSONL成功；ZIP SHA256 dd36256ca6fcd3f2dbd287bbfa64193bfea6797cdbdf59f6dc855a950bed5666，证据runs/standalone-doc-validation及build/standalone-doc-package。未调用付费模型，未修改固定vendor、比较回执或会话接受语义。

## 源码Skill、真实四图测试与知识库工具升级（前阶段，0.2.0）

本轮全部需求和验收状态持续保存在忽略的build/task-state/current-requirements.md，防止压缩上下文后遗漏；后续Agent先核对此文件，不重复发起已完成的付费绘图调用。

- 用户覆盖此前单Linux包要求：Skill现在只附生产Go源码和固定资源，不附任何平台二进制；目标机Agent按根Skill说明下载Go1.25+并首次本机构建。官方来源https://go.dev/dl/ 与https://go.dev/doc/install 。source-only exporter manifest schema2，版本0.2.0；搬移后编译/运行/再次导出已局部验证，最终清单待本轮所有改动稳定后核验。
- 用户修改的runner参数已原样迁为examples/runner.yaml.example，含中文注释；真实runner.local.yaml仅本地忽略，来源为用户授权的外部PDF2MD接入，使用gpt-6.1-sol，密钥不显示、不提交。旧JSON原样归档runs/config-migration，正式示例不再使用正常运行文件名。
- 四图全自动已真实完成：图6.9/8.8/8.9/8.10重叠分别25.010796%、62.325647%、50.890610%、35.193841%；尺寸偏差1.172448%、0.831351%、1.515310%、0.580678%；均LuaLaTeX、1次真实submit、非best-effort、无warnings。6.9及8.10保留uncertain=true；后者原扫描中部开关疑似K1，模型按原样保留并在源码注释，而非擅自改为K2。四图最终预览均已实际检查，证据runs/automatic-four-figures。此次结果代表真实模型运行；此前“未调用真实模型”均为旧阶段记录。
- 运行发现[url](images/...)普通图片链接在初始选图及image_context中各有一处旧限制，现两处均修复，并用用户精确up6/down6参数在原文图6.9第10520行验证前后图/上下文。回归同时覆盖原![...]语法。
- 根viewer.html复用DocVision专用figure_compare/viewer/viewer.html，不使用旧Python metrics viewer；只读文件夹扫描、按需加载选中会话/关联媒体，适配Go JSONL/run/result，删除截断写入功能、避免checkpoint重复；来源与MIT许可记录docs/viewer-upstream.json。已做Node逻辑与真实Chromium加载实际会话验证，浏览器选择目录句柄由测试注入，未声称自动化操作真实系统目录对话框。
- 文档工具升级包含默认search无图、模型按需PDF页/区域view、库内文件定位/直接read和高频段落缓存；自动摘要只读、不加频次，模型主动工具阅读才计数。PDF read/query/page兼容、段落合并去重/可调长度、跨平台缓存锁和完整回归仍按需求清单逐项核验，不用旧阶段结果代替新功能验收。

### 本轮交付与验证补充

四图均已从原JSONL接受快照无模型恢复，再交付为内嵌LuaDraw的submitted.tex，PDF实际由该TeX重编；best_overlap/scale_dev/uncertain原样保持，api_contacted=false表示此次重交付未触达模型，历史首次绘制确实使用gpt-6.1-sol。另把图6.9的TeX单独复制到空输入目录，CLI强制LuaLaTeX与XeLaTeX两路线均成功，后者backend明确standalone-lua+generated-TikZ。证据runs/tex-only-validation。

Go全模块test/vet及Linux/Windows编译已通过，Windows未执行。Python开发测试最初有1个模块因入口缺少dev/python/algorithms未导入，其余75项通过；补正确PYTHONPATH后该模块4项通过，合计79项；修改后的文档镜像5项再通过。真实文档payload read/query/PDF/page14、库内导航、不同snippet长度、frequent短ID选择与无图默认已在CLI验证，证据runs/final-doc-validation。目录查看器用真实会话加载和截图，测试注入只读folder handle，未操作系统原生目录对话框。频次共享对象并发精确计数与race检查通过。

### 最终源码包核验

源码包0.2.0的manifest共304个文件，分发标记source-only，Go1.25+；无二进制、真实配置或缓存，实际密钥/私有端点字节扫描无泄漏，固定LuaDraw245文件哈希一致。解压到独立目录后真实go build并使用该包程序read/query读取官方PDF第14页成功，资源不依赖checkout路径。源码ZIP位于dist/source-v0.2.0，四图TeX/PDF/会话包位于runs/automatic-four-figures；全部产物忽略，不入Git。

PDF文本索引升v4，算法版本进入指纹：分段用上一视觉行高度而非累计段bbox高度，限制每段12行/2048字符并识别保守标题边界，修复误把整页合为一段；实际page14可分段精读，旧频次坐标失效。显式list/find/grep的已返回导航文本与view的相交页面段落也计频；自动prompt摘要保持只读。

### 后续DocVision设计批注（当前不实现）

用户提出未来将全局高频与每本书高频分层：一本书内共享文档段落频次，每书最常用前10段向历史全局索引贡献；新书尚无书内记录时，使用全局高频冷启动。模型主动阅读推动频次变化，使最终知识段落有机会超过最初的目录/导航段落。自动嵌入user上下文的摘要不加次数，模型主动查看高频某条/范围及进一步读原文才贡献计数。当前独立工具只维护全局高频，不引入书籍工程、MinerU或DocVision依赖。

## 默认单Linux amd64发布（前阶段，0.1.2）

用户要求默认包只放常用Linux amd64程序，其他平台已有源码，需要时再构建。scripts/export_skill.sh和Go build命令默认目标均改为linux/amd64；保留显式--platforms及独立输出目录，跨平台构建按需触发。根VERSION/Skill metadata更新0.1.2，避免与原五平台0.1.1目录混用；旧版本产物保持历史归档。

实际默认脚本导出仅bin/linux-amd64/compare-figure一份程序，manifest仅linux-amd64、258文件；仍复用每次构建隔离目录，现存五平台缓存不会混入新包。已实际检查Go build省略platforms只生成Linux amd64；显式windows/amd64仍成功交叉编译（未执行Windows程序）。导出移动/幂等回归及app vet通过，ZIP清单/全部文件SHA/版本一致性核验，证据runs/single-platform-validation。

## LuaLaTeX缓存修复与用户复杂LuaDraw测试（前阶段，0.1.1）

用户指出当前LuaLaTeX可用，并提供准确的复杂2D/3D压力测试附件。本轮以附件为准，不再使用skills分支内嵌简单片段；两份测试原字节保存在testdata/luadraw，manifest记录完整SHA、附件名和覆盖范围，不依赖机器附件目录。

- 2D源2225bytes，SHA93706afa92a6ccbcce4fc4f67268ed49e50a472dfc68366404c2d65cdf7e4dab；3D源2124bytes，SHAaf411326f1ed64fab6a6d3ab113199ada4a45d66aeb54873b692a1e13db58547。包括patterns、plotmarks、decorations.markings、透明多边形及36×36曲面、cutfacet分层、调色板面片光照、Stealth螺旋线和数学标签。3D生成3502行TikZ。
- 已用同一LuaLaTeX命令/同一LuaDraw探针做对照：默认缓存和原分离TEXMFVAR/TEXMFCACHE/XDG目录组合失败；把三个变量指向同一可写根后成功。正式Go及Python对照统一此策略，探针和实际编译一致，缓存仅在自有临时/工作目录，不改HOME/系统缓存。此前“宿主LuaLaTeX不可用”是探针配置导致的旧结论，当前已真实验证可用且优先直编。
- 用户复杂3D回退首次暴露Unknown arrow tip kind Stealth，已在回退探针/包装中预加载LuaDraw.sty相同的patterns,patterns.meta,plotmarks,arrows.meta,bending,decorations.markings,backgrounds；不修改245个固定vendor文件。Go覆盖测试对每份附件先验SHA，再分别直编/回退，检查实际TikZ、PDF与覆盖标记；mShaded生成逐面RGB填色，不误断言必须含字面shading。
- 验证：Go各模块回归及vet通过；Python完整79项全部通过、0跳过，补齐库后运行runtime9项再次通过。新增Go直编真实测试通过；原79项中的LuaLaTeX跳过已消除。Go首次全测受源VERSION/SKILL测试中途更新影响的幂等检查，以及不准确shading字面断言，固定版本并改为实际RGB面片标记后受影响app/luadraw包复跑通过，未改生产比较回执。
- 正式运行时已更新，默认版本与根Skill元数据升至0.1.1，并重建五平台Go-only包；复杂测试只随源码测试区维护，不打进运行Skill包。仅Linux amd64本机实际执行，其他平台为交叉编译；未调用真实绘图模型。旧简单片段移到runs/rejected-simple-fixtures，不作为复杂测试交付。

## 修复Python历史小图平移失败（前阶段）

用户要求解决此前保留的小图平移失败。已复现并修复，不再作为已知失败挂账。

- 根因是dev/python/compare.py的levels_for：墨迹裁剪后画布28px、目标27格，却在没有32格层级时回退为32格。整数网格边界重复，产生零面积格子，to_grid出现invalid divide，评分被污染并选中(-2,5)错误平移。小图现在直接使用不超过画布的final格数，大图原32/64/…金字塔不变；生产Go比较及回执未改。
- 原用例正确平移仍严格断言(1,8)，且增强为100%重叠和完整图逐像素相等，并把NumPy除零/无效运算设为测试错误。附带修正原用例“至少2个起点”的错误预期：两份相同裁剪墨迹的质心重合，origin/centroid本来应去重为一次。新增不同起点确实保留两次搜索，以及24/28/31/32/33/64/96画布的正面积边界回归。
- 完整conda base Python unittest共79项：78通过、1宿主LuaLaTeX直编不可用跳过，0失败；包含GoCLI数值/JSONL和Python接受态由Go恢复的5项对照。修复仅开发原型，不需要更改Go-only发布版本或重打二进制。下方77项/1失败是修复前的历史结果。

## 仓库整理与版本化Skill发布（前阶段）

用户要求整理根目录、仓库可直接作为标准Skill、带版本号导出脚本并分批提交。正式Go实现/资源保持原位置，所有Python原型与研发算法/测试/历史查看器集中dev/python，示例配置移examples/runner.json，散落缓存仅移build/archived-cache，不删除运行结果或用户文件。

- 根SKILL.md新增name/description/license/metadata.version YAML frontmatter，兼容源码checkout与带平台bin的安装包。Go导出器从该唯一文件读正文，删除原Go内嵌Skill文案；版本由--version或根VERSION（当前0.1.0）提供，SemVer严格校验，安装包VERSION/manifest.version/metadata.version统一。
- scripts/export_skill.sh可从任意cwd使用，默认构建五平台，支持--go/GO_BIN、--platforms、--out-dir、--skip-build；输出compare-figure-skill-vVERSION目录、ZIP、SHA256。每次平台构建隔离，避免旧平台泄漏；旧输出未知文件/符号链接拒绝，不删除用户目录；安全清理本次拥有临时目录，ZIP原子替换。全新检出先创建build父目录，导出包不含Python/开发数据。
- Python迁移修正module/test项目根和prompts/vendor/scripts/.deps/build定位；完整unittest77项：75通过、1历史test_asymmetric_content_origin_compensation失败（原算法translation=-2,5而旧断言期望1,8）、1真实LuaLaTeX因宿主fontloader缓存不可用跳过。不借目录整理改历史算法。新位置GoCLI/runner恢复及LuaDraw fallback集成通过。
- Go全模块test/vet、Skill版本/正文幂等及frontmatter校验通过，脚本语法与非法版本/参数测试通过。固定vendor字节不改；Git whitespace check针对原样资源有上游尾随空白，非vendor检查通过，245个锁文件哈希均需保持。
- 最终发布验收：从仓库外cwd运行脚本，默认VERSION导出v0.1.0五平台包成功；262个manifest文件+manifest共263常规ZIP文件，逐文件SHA与ZIP校验、可执行权限、三处版本一致、Skill正文单一来源全部通过，245个锁定vendor文件均已被Git跟踪。只Linux amd64实际执行，其余交叉编译。再从仅含已提交内容的干净clone跑完整Go test/vet与首次linux/amd64版本导出均通过，证明不依赖未跟踪资源或旧build目录。证据runs/repository-validation/verification.json，版本包dist/compare-figure-skill-v0.1.0.zip与.sha256。正式发布不含Python/开发测试/运行数据。
- 分主题本地提交：657d23f原生Go+固定资源；f82431f Python归档；0a54917根Skill+版本化发布；最后同步README/接口/维护文档。没有推送远端。

## 独立尺寸CSV与Markdown上下文（前阶段）

用户明确本工具独立运行、不需要MinerU/DocVision内部工程；正式Go已删除content_list/layout/bbox/index搜索和缓存，公开CLI/JSON不再提供parse-root。旧Python原型及下方早期迁移表只作历史来源，不代表当前正式运行行为。

- 新go/measurement统一root比较API与drawing工具测量：显式mm优先，再指定/邻近sizes.csv（size.csv兼容），否则aspect_only。CSV接受注释scale_px_per_pt、BOM、quoted/相对路径；宽高mm各自直接使用，scale只保留来源不再次缩放。匹配优先完整相对路径，重名拒绝歧义；文件更新立即生效；无CSV是正常单图模式不报损坏。无内部解析文件依赖。
- root比较JSON新增text/reference_measurement/candidate_measurement/normalization；文字提供Soft-ink overlap各通道、原/候选画布mm、页宽及来源、墨迹占比/物理mm、宽高倍率和最大偏差、墨迹长宽比、应用平移/各起点、crop/输出像素/叠图图例。所有物理值在候选像素拟合前计算；拟合为等比缩到原图宽度+补白共同画布，不拉伸两个轴。drawing compare返回同样证据，提交保留overlap和同次size deviation回执。
- runner在参考图复制前保存CSV测量和页宽，比较时携带来源，避免复制到工作区后丢尺寸；显式sizes-csv与reference-mm都可用。半自动tool初始化也保存独立测量，后续沿用。默认17%/10%门槛保持。
- ResolveMarkdown与CLI接线支持Book.md+images/Book/figure.jpg+sizes.csv，run/tool/compare用source-md/image-ref选图，JSON tools compare接受source_md/image_ref；图路径以MD目录解析，支持URL编码/标题/空格/Windows分隔。单图可自动选，多图必须明确，重复basename不猜。取目标前后20行初始上下文，完整md_content/current_img/images_dir用于image_context/邻图；用户书稿不修改。外部HTTP/data图片由调用方先下载。
- 全Go `go test ./... -count=1` 与 `go vet ./...` 通过。专项完全使用临时图片/CSV/Markdown，没有任何MinerU文件：CSV 86.63×103.71/page143.93读取、候选mm两倍时overlap近1但size gate失败、不同像素同mm、无CSV aspect_only、跨调用工具、独立中文书名目录CLI/JSON、模拟模型runner真实LuaDraw编译后仍保留CSV来源。前次一项旧测试使用不存在Markdown图链接，已改成真实链接后全通过。未调用真实绘图模型。

- 最终回执边界：用户要求compare_figure文字与原版一致，已逐字采用上游figure_compare.go的Soft-ink/figureSizeSentence/formatAlignmentRuns模板，删除自行增加的画布/来源/缩放说明；Agent工具直接返回文字+图片，JSON程序接口保留结构化测量字段。新增receipt黄金用例逐字匹配用户给的14.74%/1.7%回执。尺寸策略没有另加门槛：已实读上游tools.go253-355，compare只报告，submit原本配置MaxScaleDev时会拒绝超限；原SUBMITTED/REJECTED与8次后best-effort保持。前文描述的额外文字已被此收敛替代。
- 五平台Go程序与Go-only Skill已重建，重复导出written=0/unchanged=261。开发对照test_go_cli/test_go_runner5项全部通过；没有改变重叠算法的数值契约。实际发布程序在runs/independent-validation独立目录运行图片+CSV/Markdown、翻倍mm、单图无CSV、持久image_context，验收JSON保存在该目录。仅Linux amd64实跑，其余平台交叉编译；ZIP更新为dist/compare-figure-skill-go.zip。下方双模式/完整迁移测试为前阶段记录。

## 双模式Skill与有界默认输出（前阶段）

用户要求在完整Go程序上分自动与Agent半自动两模式，默认都17%重叠。一个Skill入口已明确模式选择、完整操作顺序、图像查看和停止标准：自动run/auto由给定model/baseURL/key驱动；半自动当前Agent亲自写LuaDraw，直接调用docs/luadraw-doc-search、compile/luadraw-compile、compare，无需另一个模型。原11工具另有tool/agent-tool持久状态CLI，支持跨调用compile→compare→submit→deliver。

- 自动模式新增临时api_key/--api-key、--api-key-file；原--api-key-env及配置继续支持。模型URL/名称/key直接接入的真实编译+模拟API测试已通过，默认run.json门槛0.17；run.json/session.jsonl/result.json均未包含临时key。
- compare增加--reference/--candidate别名、百分比--threshold/--pass-threshold、尺寸门槛、参考/候选毫米、crop/detail/zoom/worst-region/inline-images。响应增加similarity_percent与thresholds的实际值/两个gate状态，固定result.json保存完整结果。比较不达标是有效结果（exit0/passfalse），不能只看进程码当验收。
- 默认输出改为OS临时根下用户/当前工作目录/操作对应固定槽，可用COMPARE_FIGURE_OUTPUT_ROOT指定根。ProcessRequest库、compare命令、JSON tools共用；同工作目录比较下一次覆盖comparison.png/metrics.json/result.json。文档CLI将唯一渲染图发布为预览/document.png后只删除本次自有渲染，重复调用不累积图片。不扫用户目录或旧版本遗留临时目录；需要保存或并行的调用显式out_dir/--out。
- 半自动直接CLI支持文档精确检索/源码read/PDFview百分比区域、真实编译与图片回传。修正未显式设置的right/bottom/page零值误传导致默认裁剪失败；数值参数只转发用户实际给定项。
- 新增Go专项验证：共享ProcessRequest重复5次只有固定3文件；半自动CLI固定输出/17%门槛/85%覆盖/显式目录/base64；Dpolyline3d真实手册定义和默认附图连续3次路径不变、文件数不变；自动直接key不落盘。
- 最终Go全模块 `go test ./... -count=1`、`go vet ./...` 通过，包含Agent工具跨调用真实write/compile/compare/submit/deliver/re-deliver、17→0.17与后续50%覆盖、Markdown上下文、工作区原图绑定、共享锁。开发对照test_go_cli/test_go_runner共5项通过，原型数值与接受态恢复兼容。未调用真实绘图模型。
- 五目标linux-amd64/linux-arm64/windows-amd64/darwin-amd64/darwin-arm64已重建；只Linux amd64本机执行。实际Skill导出261文件，重复导出written0/unchanged261；清单含modes和默认门槛，ZIP不含Python且逐文件SHA核验。发布包内程序实际执行双模式help、无key文档/3D编译/重复比较，默认17%，比较固定3文件、文档固定1图，输出路径均稳定。验收JSON在runs/modes-validation/verification.json；已更新dist/compare-figure-skill-go.zip。之前runs/go-native-validation及下节为上一版本验收记录。


## 完整原生Go runner与Go-only Skill（前阶段）

最新要求将完整Python runner再迁移为幂等、多平台Go程序。已新增go/session、go/luadraw、go/drawing、go/runner、go/app、go/distribution，根Go包新增共享ProcessRequest；命令为run/tools/compare/environment/build/export-skill。Python只作开发原型/测试，Go程序不通过Python桥接。对应关系与用法见 [go-native.md](go-native.md)。以下Python演进与“未运行Go”等文字均为历史时点记录。

- 完整迁移11工具、真实LuaDraw环境探针/编译回退、MinerU测量、RE2 grep、上下文/邻图/软预算、PDF局部高分辨率重渲染、文档查询文字+官方PDF图、提交和已接受源重编。Rasterize支持pdftoppm/pdftocairo；原始测量在拷贝参考图前保存。
- 三协议请求体/SSE、Responses图片和加密项、Anthropic图片/思考签名、Chat多工具分片、tool_choice none、模型参数与retry/backoff按原型核对。API错误脱敏；退避2/4/8/16/30秒可取消。修复了首次Probe与缓存切片类型不同触发的panic，并覆盖fresh Compile成功/失败收账。
- 幂等语义具体覆盖：相同输入自动恢复，不同输入拒绝混用；完成后不重复模型请求、不需密钥；接受后交付中断无模型重编；初始化失败重试保留原任务/图；进程锁和死锁拥有者恢复；移动工作区的产物路径重定位；模型wire名多别名按文件顺序解析，避免map迭代随机性。
- 复核真实集成走七次模拟模型请求：初稿、issues、write、compile、compare、submit、pass，确认未被旧submitted状态提前停止；无结论时保留已接受源。压缩/检查点续跑、预算grace、Unicode日志修剪和自定义Completer计轮有Go专项回归。
- 最终Go1.25.11的 `go test ./... -count=1` 和 `go vet ./...` 通过。所有绘图模型请求使用本地模拟服务；真实Lua/TeX/Poppler用于编译、尺寸、文档、图像验收，未访问真实绘图模型API。
- Python开发对照命令为 `PYTHONPATH=.:algorithms:tests python -m unittest test_runner test_vector_tools test_vector_metrics test_go_cli test_luadraw_docs test_luadraw_runtime test_luadraw_session test_tools_cli test_go_runner -v`（conda base）。结果71项：70通过、1真实LuaLaTeX宿主原因跳过。新增test_go_runner由Python实际生成接受态、在尚无result时直接交给Go无模型恢复，然后重复收账；原型分数/透明/缩放/平移/mm fixtures仍通过。
- 使用原生build命令、CGO_ENABLED=0构建linux/amd64、linux/arm64、windows/amd64、darwin/amd64、darwin/arm64五目标；已核对ELF/PE/Mach-O和架构，Linux两目标为静态ELF。仅linux/amd64在本机执行；其他四目标只有交叉编译，不能称为目标机验收。Build记录在build/platforms/build.json。
- 实际export-skill发布包在dist/compare-figure-skill，261文件及manifest；第二次导出written=0、unchanged=261。全部SHA256与245个固定vendor原文件均核验，零.py/.pyc/.pyo。Go-only ZIP为dist/compare-figure-skill-go.zip；最终条目数、字节数与SHA256记录在runs/go-native-validation/verification.json。发布归档使用标准ZIP格式，不引入Python导出选项。
- 发布包内Linux程序实际运行environment、3D compile与docs query，均返回纯JSON和有效图片；可移动导出测试在与源码无关的目录运行成功。本地验收请求/结果、PDF/预览与verification.json在runs/go-native-validation（忽略）。该目录和dist/build均为本地可重建产物，不提交个人运行数据。
- 文档发布验收额外发现Poppler XML属性大小写（xMin/yMin/xMax/yMax）误读，导致零坐标/整页合块，已改正并提升索引schema强制旧缓存失效，增加精确方法名优先和真实坐标断言；避免把通用3D提及当作方法用法。未接受任务续跑现恢复submit_attempts，保留8次门槛；对应真实runner回归通过。
- 本机LuaLaTeX的fontloader缓存不可写，程序如实标记lualatex_usable=false；已实测texlua→生成TikZ→XeLaTeX/pdfLaTeX。未验证本机不可用的直接LuaLaTeX成功分支，不宣称真实模型重绘质量或所有输入跨实现逐位一致。旧compare.py小图平移失败仍按历史记录保留，未改变旧算法。

## 原始算法提取

最初从 [DocVision](https://github.com/LingNc/DocVision) 的 `3761927db079db7e10aa9fbf560fe9ddc8960bb4` 提取，保留 MIT 版权。

- [compare.py](../dev/python/compare.py) 来自 `figure_compare/algorithms/compare.py`，保留旧版实验CLI行为；同内容Skill副本已删。
- [viewer.html](../dev/python/viewer.html) 来自上游比较结果查看器，离线查看指标与叠图。
- [旧Go算法](../go/compare.go) 及其测试仅保留图像算法，不包含原宿主会话。
- [容错指标](../dev/python/algorithms/robust_metrics.py) 与 [校准工具](../dev/python/algorithms/calibrate_metrics.py) 保留研发用途，不混入主门槛。

## Python会话演进

第一版独立runner曾参照旧实验会话重写：精简系统提示词、完整LaTeX文档输入、50%默认门槛、额外哈希门槛、仅Chat Completions、禁止局部比较授权。它只完成了独立闭环，**不是生产矢量阶段的等价移植**。该实现已被本轮迁移替代，其8项旧测试随协议变化改为新的源契约测试。

用户明确要求完整迁移矢量图阶段后，基准更新为源仓库 `20faca6aaaf8d9469b3ee68e5ff7477437b7a0d7` 的 `RunTikZSession` 生产接线。详细来源、契约映射及独立输入适配在 [migration.md](migration.md)，文件哈希在 [upstream.json](upstream.json)。

- 绘图/复核的system和user四份提示词逐字复制，以哈希测试保护，不再手写精简版。
- 完整迁移10个工具、正文片段包装和导言区提升、PDF局部高分辨率查看、原图/邻图查看、扩展上下文、view软预算、生产版比较与提交门槛。
- 默认门槛恢复生产版0.17重叠/0.10尺寸偏差/8次提交后退让；隐藏参数、动态阈值、uncertain与merges保持源语义。`compile.merges`数组累积、`submit.merges`定义字符串却未使用的不对称也保留。
- [vector_metrics.py](../dev/python/vector_metrics.py) 迁移当前生产Go算法，加入mm尺寸与aspect退化。Catmull-Rom按锁定的x/image v0.39.0实现，而非换成Lanczos；Go负整数除法、行优先积分、原始精度分数等也按源码处理。未运行Go，不宣称跨语言逐位一致。
- [vector_measure.py](../dev/python/vector_measure.py) 迁移MinerU尺寸祖先查找/根目录索引和尺寸提示；入口复制原图前保存测量，独立素材支持显式印刷尺寸。
- [vector_session.py](../dev/python/vector_session.py) 替代宿主会话服务，支持Chat/Responses/Anthropic、SSE组装/非流降级、完整工具回执后再回图、预算/压缩、媒体文件转录与续跑。
- 接受提交后持久保存现场；交付前中断不重新请求作图模型。压缩后的统计仅来自保留历史；续跑不注入开场文字或重复原图。已交付收账须仍满足当前阈值。
- 可选视觉复核将问题打回同一会话，按源策略在复核不可用/缺少结论/达到轮次上限时警告并交付当前版本。
- 为保持grep的RE2语义，添加Python google-re2依赖。工作区共享conda目录只读，本机依赖安装到被忽略的 `.deps/`，解释器仍为conda base；没有改共享环境。

旧 [compare.py](../dev/python/compare.py) 的CLI与数值行为保持不变，新生产会话走 [vector_metrics.py](../dev/python/vector_metrics.py)。这两条路径对应不同的上游版本与门槛策略；合并前必须明确兼容性，不能用旧评分器冒充当前生产行为，也不能静默改动历史CLI结果。

## 指标边界

传统主分是RGB软墨重叠均值，不等于语义正确。生产版另在归一化前量mm尺寸，未知物理尺寸时只比墨迹宽高比。候选缩放后看上去吻合，不代表印刷尺度正确。

附加指标包括严格灰度Jaccard、默认2像素半径容错覆盖和默认64格粗结构重叠。综合视觉分默认0.65×容错覆盖+0.35×粗结构重叠，未接入生产submit。

旧CLI的adaptive-v1门槛为 `clamp(base + 10*(0.5-confidence) - 12*(difficulty-0.5), min, max)`，仍是未校准启发式，与生产固定门槛无关。

## 已知问题与验证范围

旧CLI最初在本仓库与源仓库均复现：

- `test_asymmetric_content_origin_compensation` 期望平移(1,8)，实际(-2,5)，伴随小图网格除零/无效值警告。旧算法保持原实现，未借会话迁移更改它。
- 校准工具仍把 `content_ink` 返回的tuple传给 `progressive_overlap`，接口待更新。

新迁移测试覆盖提示词哈希、工具定义/操作、上下文、尺寸、软预算、隐藏退让和动态阈值、源算法边界、转录/压缩/续跑收账、三种本地模拟API及真实XeLaTeX/Poppler交付。没有读取私人配置/密钥，没有请求真实模型API，也没有运行Go。

## LuaDraw 与 Go JSON 工具升级

后续用户要求升级Go生产算法、强制LuaDraw、提供快速文档检索附图与宿主编译回退。该变更是原生产迁移之后的新策略，原四份提示词作为哈希基线仍不改；当前默认使用新增LuaDraw提示词与11工具的LuaDrawHarness。

- LuaDraw固定3.7，邻仓0.3.6实际是exam-zh；完整245个分发文件与官方PDF原字节复制，独立lock记录SHA256。无隔壁仓库运行依赖。
- 参考DocVision本地origin/feat/skills `f0c00e975588bbea10f1f3873f2cb53f9d1829c0` 的SK-I16预编译、测试与官方索引。没有切分支、改源仓库或联网拉取。参考脚本与MIT许可保留在vendor/docvision-skills。
- LuaDrawRuntime检测工具版本/宏包/模块并实际编译。优先真正可用的LuaLaTeX，否则同一LuaDraw源通过Lua/texlua生成TikZ再走XeLaTeX/pdfLaTeX。主稿可用figure.lua或单个LuaDraw环境TeX正文；拒绝纯手写TikZ，编译失败返回真实日志，不把命令存在误报成可用。
- 本机LuaLaTeX存在但luaotfload/fontloader报告缓存不可写，检测标记不可用；真实texlua→TikZ→XeLaTeX的2D/3D/中文数学标签通过。相关direct测试按原因跳过，未报成通过。
- luadraw_doc在同一调用里返回官方教程/源码/例子文字、页码位置与PDF局部图片；支持手动指定PDF区域和源码行窗口。优先PyMuPDF，可退到Poppler bbox；索引按stat fingerprint失效、跨会话共享暖缓存。MuPDF注释警告不污染JSON stdout。
- Go升级到同一20faca6生产算法；已额外核对较新origin/feat/skills的figure_compare.go，两分支Git blob均为749d735ad5166d3690aec09c83b665103435cfd5，算法源码无分支差异。独立module锁x/image0.39.0。Go1.25.11上test/vet/build成功；CLI支持单请求/JSONL、PDF/image输入、系统临时输出、路径/base64图片、全图评分后裁图、负值关闭门槛和每次PDF临时目录清理。
- tools_cli统一compare/docs/environment JSON接口；compare_backend复用一个Go子进程，未构建binary时auto用生产Python，强制Go时缺binary即报错。
- 已接受稿与后续未提交草稿分别保留；交付重新编译已接受LuaDraw编辑源并保留其同次评分。version3新目录支持接受后交付中断恢复；旧version2的TikZ会话不直接混用。
- 本机合成640×480图（5次暖运行）Go JSONL比对中位约319ms，官方文档纯文字暖查询约37ms，仅为本机fixture测量。实际证据放忽略的runs/luadraw-validation。

完整JSON契约和运行命令见 [tools.md](tools.md)。最终生产/新增验收命令：

```bash
conda run -n base env PYTHONPATH=.:algorithms:tests python -m unittest test_runner test_vector_tools test_vector_metrics test_go_cli test_luadraw_docs test_luadraw_runtime test_luadraw_session test_tools_cli -v
```

结果70项：69通过、1按真实LuaLaTeX fontloader缓存限制跳过，退出0。包含texlua→XeLaTeX与texlua→pdfLaTeX实编、Xe失败后继续PDF的选择测试、真正多轮视觉修正、单JSON编译返回PDF/inline图片，以及可重放3D编译请求。Go test/vet/build通过，固定245文件哈希核验和git diff --check通过。

仓库完整发现回归此前已复现旧compare.py小图平移失败（期望1,8，实际-2,5）；该旧实现和测试未修改，不能把相关验收全绿称作旧CLI问题已修复。没有调用真实模型API；实际模型绘图质量尚未验收。

## 维护

生产会话各模块只维护一份，避免为Skill重复复制。旧CLI是明确的兼容入口，不是新会话评分器。真实素材与运行结果放被忽略的 `runs/`，本地二进制依赖放 `.deps/`，预览集中归档。重要变更同步README、AGENTS与迁移说明；上游更新时先更新来源版本/哈希，再核对工具与测试。
