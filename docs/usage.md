# 详细使用说明

快速安装、主要功能和使用示例见 [README](../README.md)。自动 runner 启动前先按[配置指南](configuration.md)查找已有配置：显式 `--config`、`COMPARE_FIGURE_CONFIG`、当前目录、Skill资源根依次选取，标准名为 `runner.yaml`／`runner.yml`／`runner.json`。配置好模型和运行设置后只需传本次任务输入及输出目录；先用 `run --check` 查看 `config_path`，不要重复索要已有接入参数。以下保留CLI、输入尺寸、会话、工具与验证细节；命令默认从仓库根执行。

自0.3.0起支持 [luadraw／mixed／latex／asymptote四策略](drawing-modes.md)，下方LuaDraw源码示例仅代表默认策略。配置 `drawing.mode` 或 `--drawing-mode` 切换；常规TikZ在latex/mixed模式允许。文档工具通用名为 `document_search`／CLI `document-search`，旧LuaDraw别名保留，底层直接引用独立doc-library，旧缓存文件格式和频次ID不迁移。最新差异见 [兼容矩阵](doc-library-compatibility.md)；下方历史验证数字标记原时点，不作为新版本包数量。

原生 **Go LuaDraw 矢量图 runner 与图形比对工具**。完整会话、模型协议、11个绘图工具、文档检索附图、编译、复核、续跑和交付均在Go中实现。**Skill只导出Go运行版本；Python只用于开发原型和对照测试，不是运行桥接或导出选项。**

生产比较/会话基线为DocVision `20faca6`，默认绘图资源为LuaDraw 3.7，另支持三种绘图库策略。当前四模式使用[完整文件提示词](prompt-guide.md)，两份旧unused绘图模板已移除、两份原版复核模板保留；历史来源哈希仍用于追溯。来源和许可见 [清单](../docs/upstream.json) 与 [固定依赖](../vendor/dependencies.lock.json)。完整迁移、接口及幂等行为见 [Go说明](../docs/go-native.md)。查看器的公式与代码显示规则见 [离线数学排版](viewer-math.md)。

## 原创LuaDraw图与独立文档工具

没有参考图时，当前Agent也可使用本Skill：先查文档，再编写LuaDraw程序、编译并查看PDF，按用户要求修正；不需要模型密钥、书稿或参考图，不进入比对/submit流程。

```bash
build/compare-figure luadraw-doc --mode frequent --cache-dir work/shared-doc-cache
build/compare-figure luadraw-doc --query Dpolyline3d --cache-dir work/shared-doc-cache
build/compare-figure luadraw-doc --request examples/luadraw-doc.json.example
build/compare-figure luadraw-doc --stdin --cache-dir work/shared-doc-cache
build/compare-figure luadraw-doc --jsonl --cache-dir work/shared-doc-cache
build/compare-figure compile --source figure.tex --out work/compiled
```

专用文档入口接受裸参数JSON或docs envelope，stdout为JSON；JSONL常驻复用索引实例，错误逐行返回后继续。沿用七种文档模式、稳定段落ID、高频计数、无图搜索与按需PDF查看；缓存可通过CLI/JSON/环境变量指定。现有 `docs`、`luadraw-doc-search` 和 `tools --jsonl` 仍兼容。CLI及外部程序/Go包调用、缓存优先级、退出码见 [独立文档工具说明](../docs/luadraw-doc.md)。索引内容是LuaDraw官方手册/源码，文档操作需要Poppler，不需要LuaLaTeX；真正绘图编译才需要Lua/TeX。

## 仓库与Skill入口

仓库自身可作为Skill：根目录 [SKILL.md](../SKILL.md) 含标准YAML frontmatter（name、description、license、metadata.version），是仓库与安装包共用的唯一技能说明源。[VERSION](../VERSION) 是默认发布版本，当前为0.2.2。

```text
SKILL.md / VERSION       Skill入口与发布版本
README.md / AGENTS.md    使用说明与维护约定
go/                      正式Go程序、包和原生测试
prompts/                 固定来源及LuaDraw提示词
vendor/                  固定LuaDraw资源、许可和SHA锁
scripts/                 Lua辅助与构建/发布脚本
examples/                脱敏配置示例
docs/                    接口、迁移来源及开发记录
dev/python/              Python原型、研发算法、历史查看器与对照测试
build/ / dist/ / runs/   忽略的构建、发布和运行产物
```

源码仓库和导出的Skill包均附带Go源码，**不附带任何预构建程序**。目标机器的Agent首次使用时用Go 1.25+构建本机程序，后续复用；安装包不含Python、开发测试、真实配置或运行数据。

## 独立图片、尺寸与Markdown上下文

程序只需要图片，可另附尺寸CSV和Markdown；不读取MinerU索引/bbox或依赖DocVision工程。常见目录可直接使用：

```text
Book.md
images/Book/figure.jpg
images/Book/sizes.csv
```

```bash
# 自动：从Markdown选图，自动读旁边的sizes.csv并取图前后文
build/compare-figure run --source-md Book.md --image-ref images/Book/figure.jpg \
  --out work/auto --model MODEL --base-url URL --api-key-env KEY_ENV

# 半自动：同样从Markdown选原图，与Agent自己生成的PDF比较
build/compare-figure compare --source-md Book.md --image-ref images/Book/figure.jpg \
  --candidate redrawn.pdf
```

也可直接传 `--reference reference.jpg` 或 `--original reference.jpg`。Markdown相对链接以MD目录解析；多图时指定完整 `--image-ref`（唯一文件名也可），初始上下文取目标前后20行，完整MD供image_context查相邻图和扩展上下文。半自动 `tool`首次同样支持source-md/image-ref，无需重复original。用户书稿只读。

尺寸优先级：显式 `--reference-mm W,H` > 指定 `--sizes-csv FILE` / 图片旁或上级自动发现的sizes.csv（兼容size.csv）> 无毫米时仅墨迹宽高比。CSV格式：

```csv
# scale_px_per_pt=1.1361
file,width_mm,height_mm,page_w_mm
figure.jpg,86.63,103.71,143.93
```

CSV的mm已是定标结果，直接使用宽高，不再除一次scale。返回reference_measurement/candidate_measurement包含画布毫米、页宽、来源、CSV位置；text逐字采用原版compare_figure模板，提供Soft-ink overlap、墨迹占比/物理mm、宽高倍率/偏差、长宽比、平移和叠图图例；Agent compare_figure工具直接返回该文字和图片，不附自建JSON包。无尺寸时明确aspect_only，不误称已核验毫米。

**物理尺寸先测、像素再归一化**：候选PDF读取真实页面毫米，先计算墨迹尺寸及偏差，再等比缩放候选图到原图宽度、补白到共同像素画布做重叠比较；不会独立拉伸宽高。即使归一化后图形相同，实际印刷尺寸放大两倍仍不能通过尺寸门槛。

## 两种工作模式

**自动模式**：用户提供支持视觉与工具调用的模型、base URL和key，Agent启动runner，由runner完成绘图迭代。默认17%重叠门槛，保留10%尺寸偏差检查。无需先写配置文件也可直接接入：

```bash
# FIGURE_MODEL_KEY中放用户提供的key；也支持 --api-key-file 或 --api-key
build/compare-figure run --original reference.png --out runs/auto \
  --model MODEL --base-url BASE_URL --api-key-env FIGURE_MODEL_KEY
```

**半自动模式**：当前Agent自己看原图、写LuaDraw、查手册、编译、查看对比图并改进，不需要额外模型或key。Skill明确指导Agent在 `pass=true` 且目视确认内容、标注和结构正确后结束；直接循环无需启动runner或调用submit。

```bash
build/compare-figure luadraw-doc-search Dpolyline3d
# Agent使用自己的文件工具编写figure.lua
build/compare-figure compile --source figure.lua --out runs/manual-compiled
build/compare-figure compare --reference reference.png \
  --candidate runs/manual-compiled/figure.pdf --threshold 17
```

编译PDF路径以输出JSON的 `pdf` 为准。比较回传 `similarity_percent`、`thresholds`、`pass` 和 `images`；读取实际对比图再决定如何修改。`--threshold`单位为百分比，JSON的 `pass_overlap` 单位为比例；默认分别为17和0.17。`--max-scale-dev`默认0.10，整图打分后才裁切证据。成功计算但不达标仍退出0，Agent应检查 `pass`。

原11工具也可通过持久化的 `tool` / `agent-tool` 逐项调用：首次传 `--workspace`、`--original`、`--name`、`--args`；后续只需工作区和工具调用。`write_file/edit_file/read_file/grep/compile/view_pdf/compare_figure/submit/image_context/view_image/luadraw_doc` 均复用原生实现；`--name deliver`交付已接受稿。复杂参数用 `--args-file`。

**默认输出固定复用**：compare/docs/compile未传 `--out` 时，按当前工作目录和操作类型使用一个固定临时槽，下一次覆盖上一次。比较固定保存comparison.png、metrics.json、result.json；文档图固定为预览/document.png，重复调用不会积累时间戳图。可用 `COMPARE_FIGURE_OUTPUT_ROOT`改位置。需要保留成品时指定 `--out` 或复制出来，并行任务使用不同输出目录。自动/持久工具工作区保留有意记录的源码与状态。

## 构建与启动

源码Skill首次使用需Go 1.25+，Agent在目标机本地构建当前平台程序后复用。Go官方下载与安装说明：https://go.dev/dl/ 、https://go.dev/doc/install 。绘图需要实际可用的LuaLaTeX，或独立Lua/texlua加XeLaTeX/pdfLaTeX。文档/PDF操作需要Poppler的 `pdftotext`、`pdfinfo`、`pdftoppm`。Go文档工具不依赖PyMuPDF。

```bash
# 在 go/ 目录
GOCACHE="$PWD/../build/go-cache" go build -o ../build/compare-figure ./cmd/compare-figure

# 以下在项目根目录运行；Windows用对应 .exe
build/compare-figure environment
build/compare-figure run --original /path/to/reference.png \
  --context-file /path/to/context.md --out runs/figure-001 \
  --config runner.yaml
```

模型必须支持图片和工具调用。[带注释YAML配置样例](../examples/runner.yaml.example) 支持JSON/YAML、模型注册表和extends、Chat Completions/Responses/Anthropic、SSE/非流降级、思考参数及API重试。密钥可从指定环境变量、key文件或临时参数读取，不写入运行元数据、Skill包或转录。

`run --check`校验输入和真实编译路线，不调用模型、不创建会话目录。`environment`报告版本、包路径和实际编译可用性，不能把命令存在当作可用。当前宿主LuaLaTeX已实测可用。探针和编译使用同一套工作目录内的TeX缓存根，避免分离TEXMFVAR/TEXMFCACHE时的fontloader误判；不改用户HOME或系统缓存。LuaLaTeX优先，独立Lua→TikZ→XeLaTeX/pdfLaTeX回退仍保留；目标机器会重新探测。

## 会话、幂等与续跑

```bash
# 相同输入重复执行会自动恢复已有任务
build/compare-figure run --original reference.png --out runs/figure-001 --config runner.yaml

# 也可只复用存储的输入
build/compare-figure run --out runs/figure-001 --resume --config runner.yaml
```

- 输入指纹拒绝把不同任务静默写入同一目录。已完成且仍满足当前门槛的任务直接返回已有产物，不需要再次调用模型或提供模型密钥。
- 接受后、交付前中断可从已记录状态重编和收账。初始化失败后的重试仍保留首轮原图/任务；已开始的续跑不重复开场消息。
- 11个工具包括写/改/读/grep、compile、view_pdf、compare_figure、submit、image_context、view_image、luadraw_doc。原图与邻图通过专门看图工具读取；文件工具限制在工作区。
- 模型优先写可编辑LuaDraw程序，也接受单个真实LuaDraw环境的TeX正文；拒绝纯手写TikZ主稿。LuaDraw自动生成的TikZ是编译产物。
- 默认门槛为17%重叠、10%尺寸偏差，8次真实提交后才告知尽力还原入口。比较先量物理尺寸、再归一化，始终先整图评分再裁证据。毫米尺寸未知时只检查墨迹宽高比。
- 可配置原图印刷尺寸、局部PDF重渲染、看图软预算、动态逐图门槛、跨页merges、轮次预算、压缩、可选视觉复核。修正可跨多轮工具；复核失败交付已接受稿，交付重编该稿以避免未提交草稿混入。
- 输出含编辑源、包装/生成依赖、PDF、预览、结果JSON及文件媒体转录；同一工作区有进程锁防止并发写入。当前版本3的原型转录/元数据可继续读取。

## 原生JSON工具

```bash
build/compare-figure tools --request request.json
build/compare-figure tools --jsonl
build/compare-figure compare --original reference.png --pdf candidate.pdf --out runs/comparison
```

`tools`支持 `compare`、`docs`、`compile`、`environment`、`run`；stdout只有JSON。JSONL持续服务并复用缓存，坏请求独立回错后继续。未给输出目录则复用当前工作目录对应的固定临时槽。图片按 `images[{path,mime,width,height,base64?}]` 回传，可用 `inline_images:true`直接交给图像感知调用方。

```json
{"operation":"docs","arguments":{"query":"Dpolyline3d"}}
{"operation":"docs","arguments":{"mode":"view","doc":"doc/luadraw-doc-en.pdf","page":91,"left":10,"top":15,"right":85,"bottom":90},"inline_images":true}
```

文档搜索默认只返回用法、源码/例子位置、PDF页码/bbox，不自动附图。模型需要查看时主动调用 `mode:view`，指定检索到的doc和page即可看整页，或附百分比区域和zoom局部重渲染。搜索bbox单位为PDF点，裁切参数为百分比；显式 `include_image:true` 可按需给搜索附图。暖查询不重新抽整套手册。更多请求示例和库接口见 [Go说明](../docs/go-native.md)。

文档库还提供 `list/find/grep/read/frequent`：先列真实路径或按文件名定位，再顺着include关系精读；`read`兼容PDF的 `doc`/`query` + `page`，不自动渲染图片。检索按完整段落合并去重，`snippet_chars`调每条长度、`max_chars`调总预算。高频模式可按当前排名行或稳定 `entry_id` 选择；模型主动查看才贡献计数，自动追加到新会话上下文的摘要不计数。默认摘要最多8行、每行1024字符、总计4096字符，可在示例的 `docs.frequent` 配置中关闭。缓存放本Skill的 `build/doc-cache`，不随包分发。详细参数见 [Go说明](../docs/go-native.md)。

最终交付的 TeX 内嵌接受的 `\\begin{luadraw}...\\end{luadraw}` 代码，可以单文件编辑，不依赖旁边的Lua或生成TikZ输入；额外保留原接受Lua。`editable_tex`/`submitted_tex` 指向TeX，PDF由该TeX重编。没有LuaLaTeX仍可用独立Lua/texlua生成TikZ，再交给XeLaTeX或pdfLaTeX；PDF阶段仍需要TeX引擎。

## 带版本号的Go源码Skill导出

```bash
# 从VERSION读取版本，导出源码Skill目录、ZIP和SHA256
./scripts/export_skill.sh
# 显式版本及本机Go工具链
./scripts/export_skill.sh --version 0.2.2 --go /path/to/go
```

默认输出 `dist/compare-figure-skill-v0.2.2/` 及同名ZIP/SHA256。包只附Go生产源码、go.mod/go.sum、根Skill说明、版本、脱敏YAML示例、提示词、固定LuaDraw资源/手册、Lua辅助脚本和许可证；不附预构建程序。目标机Agent按根Skill说明安装Go 1.25+后，在包根运行：

```bash
go -C go build -o ../build/compare-figure ./cmd/compare-figure
# Windows本机：
go -C go build -o ../build/compare-figure.exe ./cmd/compare-figure
```

Go默认构建当前机器平台；首次可能下载go.mod中的模块依赖，后续复用本地缓存。其他平台不必预先打包程序；开发时仍可显式使用 `build --platforms GOOS/GOARCH` 交叉构建。发布脚本只构建本机导出工具，工具本身不入包。不同版本分别保留；同版本拒绝混入旧未知文件。VERSION、manifest版本和Skill元数据一致。Python、开发测试、真实配置、密钥与运行数据均不导出。

## 配置示例与本地配置

```bash
cp examples/runner.yaml.example runner.yaml
# 编辑runner.yaml，填写模型和base_url；密钥建议使用api_key_env。
build/compare-figure run --original reference.png --out runs/figure-001 --config runner.yaml
```

正式示例使用 `.yaml.example` 后缀并带字段注释；JSON仍兼容。`runner.yaml`、`runner.yml`、`runner.json`、`*.local.yaml`/`*.local.yml`/`*.local.json`和运行目录均由Git忽略。真实配置不纳入Skill清单；请勿把密钥填写到示例。会话/看图/编译/接受配置保留用户调整的参数（128轮、131072输出token、256000上下文等）。

## 本地会话查看器

使用 Chromium、Chrome 或 Edge 打开 [viewer.html](../viewer.html)，点击“选择会话文件夹”。选父目录可列出多个会话，也可直接选择单个会话目录。页面持有浏览器只读目录/文件句柄，扫描仅枚举路径，不复制整套文件、不上传、不修改会话，无需服务器。选择会话或点击“刷新”会取得当前文件新快照；刷新还会重扫目录以发现新增会话和图片，按路径保留选中项。当前是按需/手动刷新，不是自动文件监听。

v0.2.2修复了旧File缓存导致旧转录、历史比较图预读、切换竞态和图片URL未释放。首载只读选中转录、少量元数据和当前比较/最终图；消息里的图片进入视口才读取，折叠的工具/思考正文展开后才排版，最新比较轮次优先且缺失通道禁用。时间线默认最近100条，可点“载入更早”回看；完整图像适屏缩放和窄屏布局已修复。查看器复用用户指定的DocVision专用UI，来源与许可见 [来源记录](../docs/viewer-upstream.json)，支持Go工具调用/回执、最终预览、比较证据和checkpoint，避免重复渲染压缩快照。

Chromium受控测试（100个会话、所选约2.99MB/4000消息）：首屏DOM由168702降至1485个；20轮比较的测试会话首载读取由44降至5个文件，其他会话正文读取0。另用真实8.8会话验证41次工具和10/10张图片按需显示。测试通过注入目录句柄完成，时间不代表磁盘吞吐。仍需遍历目录路径，并完整读取/解析所选JSONL；没有流式尾读或全量虚拟滚动，超大单会话和巨大目录仍有CPU/内存开销。详细证据与局限见 [开发记录](../docs/notes.md)。

## 四图真实自动测试

使用用户指定的 `gpt-6.1-sol`，在《高等数学释疑解难》选取图6.9（立体）和8.8、8.9、8.10（三电路），均由自动runner绘制和提交，未人工修改图稿。四图均LuaLaTeX直编、一次实际submit、非尽力接受、无运行警告：

| 图 | 重叠 | 尺寸偏差 | 模型不确定标记 |
|---|---:|---:|---|
| 6.9 | 25.01% | 1.17% | 有 |
| 8.8 | 62.33% | 0.83% | 无 |
| 8.9 | 50.89% | 1.52% | 无 |
| 8.10 | 35.19% | 0.58% | 有 |

四份最终预览均已实际检查；图8.10中部开关在扫描上疑似K₁，模型保留原样并注明不确定。17%是像素接受门槛，仍应结合原图检查语义与投影。完整运行及原始结果位于本地忽略的 `runs/automatic-four-figures/`，不进入源码Skill。回归期间发现的工具错误保留于历史转录，不通过重跑付费模型来覆盖历史。

## 复杂LuaDraw回归包

用户提供的 [2D复杂测试](../testdata/luadraw/luadraw-test-2d.lua) 与 [3D复杂测试](../testdata/luadraw/luadraw-test-3d.lua) 原样保留，来源、完整SHA256与覆盖列表在 [测试包说明](../testdata/luadraw/README.md)。包括曲线/网格、图案、plotmarks、装饰箭头、透明多边形、36×36曲面、cutfacet分层、调色板面片光照、螺旋线和数学标注。

```bash
# 在go/目录：两份复杂样例各测LuaLaTeX直编与独立Lua回退
GOCACHE="$PWD/../build/go-cache" go test ./luadraw \
  -run 'TestLuaDrawComplex(2D|3D)$' -count=1 -v
```

测试先校验附件SHA，再检查真实TikZ和PDF；回退包装加载LuaDraw样式需要的patterns、plotmarks、arrows.meta及decorations等库。测试包位于源码仓库，不打入运行Skill包。

通用会话框架复用的静态调查见 [pi-go评估](../docs/pi-go-evaluation.md)：优先考察可导入的agent/model接口，保留现有接受态、媒体和旧会话恢复；本轮没有添加该依赖或迁移生产会话格式。

## 开发与验证

```bash
# 在 go/ 目录，执行原生回归和静态检查
GOCACHE="$PWD/../build/go-cache" go test ./...
GOCACHE="$PWD/../build/go-cache" go vet ./...
```

测试使用本地模拟模型API及真实TeX/Poppler，不调用付费或真实模型。跨平台构建不等于Windows/macOS实机验收，具体实测范围记录在 [开发记录](../docs/notes.md)。

Python开发原型、算法和测试集中在 [dev/python/](../dev/python/README.md)，不参与正式运行。对照测试使用conda base及 [开发依赖](../dev/python/requirements.txt)：

```bash
conda run -n base env PYTHONPATH=dev/python:dev/python/algorithms:dev/python/tests \
  python -m unittest discover -s dev/python/tests -v
```

Python历史比较器的小图平移失败已修复：小图金字塔网格不再超过画布尺寸，避免零面积单元污染评分；测试核验精确平移和逐像素对齐，正式Go评分器未改。历次验证与校准接口边界见开发记录。旧生产迁移表在 [迁移说明](../docs/migration.md)，原型JSON说明在 [原型接口](../docs/tools.md)。项目采用 [MIT许可](../LICENSE)，LuaDraw分发保留LPPL 1.3c+。
