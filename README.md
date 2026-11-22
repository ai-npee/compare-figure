# compare-figure

原生 **Go LuaDraw 矢量图 runner 与图形比对工具**。完整会话、模型协议、11个绘图工具、文档检索附图、编译、复核、续跑和交付均在Go中实现。**Skill只导出Go运行版本；Python只用于开发原型和对照测试，不是运行桥接或导出选项。**

生产比较/会话基线为DocVision `20faca6`，绘图固定LuaDraw 3.7。四份原始提示词保留哈希基线，当前绘图使用新增的LuaDraw提示词；来源和许可见 [清单](docs/upstream.json) 与 [固定依赖](vendor/dependencies.lock.json)。完整迁移、接口及幂等行为见 [Go说明](docs/go-native.md)。

## 仓库与Skill入口

仓库自身可作为Skill：根目录 [SKILL.md](SKILL.md) 含标准YAML frontmatter（name、description、license、metadata.version），是仓库与安装包共用的唯一技能说明源。[VERSION](VERSION) 是默认发布版本，当前为0.1.2。

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

源码仓库用Go构建后运行；导出的安装包自带平台程序，不要求Python或Go编译器。安装包不含dev/、测试或运行数据。

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

开发构建需Go 1.25+；已导出Skill使用原生二进制，不需要Go编译器。绘图需要实际可用的LuaLaTeX，或独立Lua/texlua加XeLaTeX/pdfLaTeX。文档/PDF操作需要Poppler的 `pdftotext`、`pdfinfo`、`pdftoppm`。Go文档工具不依赖PyMuPDF。

```bash
# 在 go/ 目录
GOCACHE="$PWD/../build/go-cache" go build -o ../build/compare-figure ./cmd/compare-figure

# 以下在项目根目录运行；Windows用对应 .exe
build/compare-figure environment
build/compare-figure run --original /path/to/reference.png \
  --context-file /path/to/context.md --out runs/figure-001 \
  --config /path/to/model.json
```

模型必须支持图片和工具调用。[配置样例](examples/runner.json) 支持JSON/YAML、模型注册表和extends、Chat Completions/Responses/Anthropic、SSE/非流降级、思考参数及API重试。密钥可从指定环境变量、key文件或临时参数读取，不写入运行元数据、Skill包或转录。

`run --check`校验输入和真实编译路线，不调用模型、不创建会话目录。`environment`报告版本、包路径和实际编译可用性，不能把命令存在当作可用。当前宿主LuaLaTeX已实测可用。探针和编译使用同一套工作目录内的TeX缓存根，避免分离TEXMFVAR/TEXMFCACHE时的fontloader误判；不改用户HOME或系统缓存。LuaLaTeX优先，独立Lua→TikZ→XeLaTeX/pdfLaTeX回退仍保留；目标机器会重新探测。

## 会话、幂等与续跑

```bash
# 相同输入重复执行会自动恢复已有任务
build/compare-figure run --original reference.png --out runs/figure-001 --config model.json

# 也可只复用存储的输入
build/compare-figure run --out runs/figure-001 --resume --config model.json
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
{"operation":"docs","arguments":{"query":"Dpolyline3d","include_image":true},"inline_images":true}
```

一次文档调用即可返回用法、源码/例子位置、PDF页码/bbox及局部图片，也可指定页码和百分比区域重渲染。暖查询不重新抽整套手册。更多请求示例和库接口见 [Go说明](docs/go-native.md)。

## 带版本号的Go-only Skill导出

```bash
# 从VERSION读取版本，只构建Linux amd64并导出目录、ZIP和SHA256
./scripts/export_skill.sh

# 显式版本及Go工具链（也可设置GO_BIN）
./scripts/export_skill.sh --version 0.2.0 --go /path/to/go

# 需要其他平台时从源码构建；单独目录避免与默认包混用
./scripts/export_skill.sh --platforms windows/amd64 --out-dir dist/windows-amd64
# 只复用默认Linux amd64的已准备二进制
./scripts/export_skill.sh --skip-build
```

脚本可以从任意当前工作目录调用，支持带空格路径。默认发布到 `dist/compare-figure-skill-v0.1.2/`、同名 `.zip` 和 `.zip.sha256`，用 `--out-dir` 改目录。版本采用SemVer，可带v前缀；不同版本分别保留，同版本可重复导出。`--skip-build`要求已经准备好build/compare-figure和build/platforms，不会自动编译。安装包中的VERSION、manifest.json.version与SKILL.md的metadata.version保持一致；发布时覆盖版本不会修改源码VERSION。

底层也可以在Linux amd64宿主直接导出本机程序：

```bash
build/compare-figure export-skill --version 0.1.2 \
  --out dist/compare-figure-skill-v0.1.2
```

默认发布只带 `bin/linux-amd64/compare-figure` 一个程序。其他平台保留源码和 `build --platforms GOOS/GOARCH`/脚本 `--platforms` 按需构建；显式 `--binaries-dir` 时按调用方准备的目录收集二进制。导出包括校验过Go构建信息/平台的二进制、根SKILL.md、VERSION、原始提示词、固定LuaDraw运行库/官方手册、Lua辅助脚本与许可证。排除Python原型、开发测试、本地配置、密钥和运行数据。相同输入重复导出跳过未变化文件，生成相同SHA256清单；包可离开源码目录运行。


## 复杂LuaDraw回归包

用户提供的 [2D复杂测试](testdata/luadraw/luadraw-test-2d.lua) 与 [3D复杂测试](testdata/luadraw/luadraw-test-3d.lua) 原样保留，来源、完整SHA256与覆盖列表在 [测试包说明](testdata/luadraw/README.md)。包括曲线/网格、图案、plotmarks、装饰箭头、透明多边形、36×36曲面、cutfacet分层、调色板面片光照、螺旋线和数学标注。

```bash
# 在go/目录：两份复杂样例各测LuaLaTeX直编与独立Lua回退
GOCACHE="$PWD/../build/go-cache" go test ./luadraw \
  -run 'TestLuaDrawComplex(2D|3D)$' -count=1 -v
```

测试先校验附件SHA，再检查真实TikZ和PDF；回退包装加载LuaDraw样式需要的patterns、plotmarks、arrows.meta及decorations等库。测试包位于源码仓库，不打入运行Skill包。

## 开发与验证

```bash
# 在 go/ 目录，执行原生回归和静态检查
GOCACHE="$PWD/../build/go-cache" go test ./...
GOCACHE="$PWD/../build/go-cache" go vet ./...
```

测试使用本地模拟模型API及真实TeX/Poppler，不调用付费或真实模型。跨平台构建不等于Windows/macOS实机验收，具体实测范围记录在 [开发记录](docs/notes.md)。

Python开发原型、算法和测试集中在 [dev/python/](dev/python/README.md)，不参与正式运行。对照测试使用conda base及 [开发依赖](dev/python/requirements.txt)：

```bash
conda run -n base env PYTHONPATH=dev/python:dev/python/algorithms:dev/python/tests \
  python -m unittest discover -s dev/python/tests -v
```

Python历史比较器的小图平移失败已修复：小图金字塔网格不再超过画布尺寸，避免零面积单元污染评分；测试核验精确平移和逐像素对齐，正式Go评分器未改。历次验证与校准接口边界见开发记录。旧生产迁移表在 [迁移说明](docs/migration.md)，原型JSON说明在 [原型接口](docs/tools.md)。项目采用 [MIT许可](LICENSE)，LuaDraw分发保留LPPL 1.3c+。
