# 文档检索工具与 LuaDraw 兼容入口

文档索引、段落、PDF 抽取与频次现由独立 [doc-library](https://github.com/LingNc/doc-library) 固定上游依赖提供。`document_search`／CLI `document-search` 是通用入口，旧 `luadraw_doc`、`luadraw-doc` 和 Go `luadraw.NewDocs` 保留。默认仍检索 LuaDraw；`--drawing-mode asymptote` 检索 Asymptote，`--drawing-mode mixed` 同时提供两者，`--doc-source luadraw|asymptote` 可筛选来源。下方旧命令示例仍适用，也可将命令名换为 `document-search`。

该工具不要求参考图、Markdown 书稿、自动绘图会话或模型密钥。新增 Go `documents.New(resourceRoot,cacheDir,mode)` 与 `NewConfig` 可配置多个文档根；宿主 `documents.New` 默认使用输出 v2；结构化 `ExecuteResponse` 以 `results` 承载正文，不重复输出 `text/cache/provenance`。显式 v1 保留旧字段。模式配置见 [绘图库说明](drawing-modes.md)，完整功能对照、输出和缓存差异见 [兼容矩阵](doc-library-compatibility.md)。默认资料不是任意 TeX 包的手册全集。

## 原创图工作方式

1. 用户说明想画的对象、标签、布局和用途，当前 Agent 分析构造。
2. 先调用 `frequent` 复用高频知识；缓存为空则继续 `search`。用 `list/find/grep` 定位实际文档和依赖，用 `read` 精读 API/示例。
3. 需要理解官方图示时才 `view` 指定 PDF 页或区域；普通查询默认不附图。
4. Agent 写 LuaDraw `.lua` 或内嵌 `luadraw` 环境的 `.tex`，调用 `compile`，查看 PDF 和预览，按用户要求修正。
5. 交付可编辑源码和 PDF。没有参考图时不报告比对分数，也不需要进入参考图 `submit` 接受流程。

曲面网格和相交场景的渲染选择、解析裁切示例与适用边界，见[LuaDraw 曲面渲染实践](luadraw-surface-rendering.md)。它同时以 `luadraw-notes` 来源接入工具：这份项目补充不是官方手册，不改动固定 vendor，也不索引整套项目文档或运行目录。

引用源由 [LuaDraw 配置](document-sources/luadraw.yaml)、[mixed 配置](document-sources/mixed.yaml) 和 [Asymptote 配置](document-sources/asymptote.yaml) 声明，都是标准 doc-library `Config` YAML；相对 `root` 按 YAML 所在目录解析。`documents.New` 按绘图模式加载配置并调用 `NewConfig`；源 YAML 未指定 `output.version` 时补宿主默认 2，显式配置的版本保留。没有写死经验文件名、来源别名或扩展筛选规则。后续补充资料只需改对应 YAML 的 `sources/include` 和增加实际文档；若使用导出包，也应将新增文档列入资源导出清单。

不传 `source` 就搜索当前绘图模式配置的全部资料；`source:"luadraw"` **只查官方来源**。同时查询官方及补充可用 `sources:["luadraw","luadraw-notes"]`。返回结果保留真实来源，后续 `read/view` 使用该精确来源。显式 `NewConfig` 继续遵守调用方自己的核心配置，包括输出选择；未指定版本时保持 doc-library 核心默认 v1，不补宿主 v2；旧版或独立 LuaDraw 目录缺少 YAML 时退回原官方来源，无效的已选配置则报错而不静默回退。

```json
{"mode":"search","query":"Dscene3d 网格 毛刺","limit":5}
{"sources":["luadraw","luadraw-notes"],"mode":"search","query":"cutfacet2","limit":5}
{"source":"luadraw-notes","mode":"read","doc":"luadraw-surface-rendering.md","start_line":1,"end_line":57}
```

配置和参考原文一起随源码 Skill 导出，安装包也可检索、精读这些经验；不是只在仓库里的链接。

## 构建和依赖

Skill 只附 Go 源码。安装 Go 1.25+（[下载](https://go.dev/dl/)、[安装](https://go.dev/doc/install)），在包根目录运行：

```bash
go -C go build -o ../build/compare-figure ./cmd/compare-figure
```

Windows 输出名使用 `../build/compare-figure.exe`。首次构建会下载固定 doc-library 等 Go 模块，之后运行不需要旁边的独立仓库。文本源码查询不需 Poppler；PDF 文字检索／读页需 `pdftotext`，显式图片预览需 `pdftoppm`。文档读取不要求 LuaLaTeX、Asymptote 或模型密钥。绘图编译按所选策略另需相应程序；LuaDraw 保留原两路线。

从其他项目调用时使用可执行程序的绝对路径。资源自动从 Skill 根定位，或用 `--resource-root` 指定 Skill 根；`--luadraw-root` 指定固定库根。文档内容只能在库根内访问，不开放任意宿主文件。

## 工具模式与目录导航

`document_search` 提供七种常用模式：`search` 正文检索，`read` 源码按行或PDF按页读文字，`view` PDF整页/区域看图，`list` 列目录，`find` 按文件名/通配符找路径，`grep` 精确文本或正则搜索，`frequent` 高频段落。目录导航限定配置文档源，不是操作系统当前工作目录，也没有跨请求保留的 `cd` 状态。

```json
{"source":"asymptote","mode":"list","doc":".","limit":30}
{"source":"asymptote","mode":"list","doc":"doc","limit":30}
{"source":"asymptote","mode":"find","query":"*asymptote.texi*"}
```

`list` 默认列直接子项，包含文件和文件夹；文件夹以 `/` 结尾，JSON有 `kind:"directory"`，`recursive:true` 可递归列允许的文档。结果受 `limit`/`offset` 分页；空目录或不含允许文档的目录不会被索引展示。缺路径的错误提示只是最多32项建议，目录优先，并非完整目录清单。

Asymptote 手册实际路径是 `doc/asymptote.texi`，不是根目录的 `asymptote.texi`；可 `find` 定位后再 `read`。`read` 给了 `doc` 和行范围就读那段，不会根据 `query` 自动把错误路径或不相关行号修正为匹配段落。先 `search`/`grep` 查 `orthographic` 获得真实路径/行号再读。

## CLI 参数调用

`luadraw-doc` 是独立入口，`docs`、`luadraw-doc-search` 保持兼容；所有正常结果为 JSON。

```bash
build/compare-figure luadraw-doc --mode frequent --cache-dir work/shared-doc-cache
build/compare-figure luadraw-doc --query Dpolyline3d --snippet-chars 1200 --max-chars 6000 --cache-dir work/shared-doc-cache
build/compare-figure luadraw-doc --mode list --doc doc/src/body-en --limit 20
build/compare-figure luadraw-doc --mode find --query '*doc3d-en*'
build/compare-figure luadraw-doc --mode grep --doc files --query Dpolyline3d --limit 5
build/compare-figure luadraw-doc --mode read --query doc/luadraw-doc-en.pdf --page 14
build/compare-figure luadraw-doc --mode read --doc files/luadraw_graph3d.lua --start-line 1 --end-line 80
build/compare-figure luadraw-doc --mode view --doc doc/luadraw-doc-en.pdf --page 14 --left 5 --top 10 --right 95 --bottom 80 --out work/manual-view
```

源码按行读取最多400行，PDF按页/段落块定位。模型文字使用 `source · doc · p.N` 或 `source · doc · Lstart-end` 定位，不显示缓存/哈希首行、PDF 点坐标或页面尺寸。`snippet_chars` 控制每个片段，`max_chars` 控制正文总预算（位置及 warning 另计）。默认不输出 bbox；需要区域定位时传 `include_bbox:true` 或 CLI `--include-bbox=true`，只补 `bbox_pct`（left/top/right/bottom 百分比），可直接用于 `view` 裁切。省略 `include_bbox` 沿用启动默认，显式 true/false 覆盖本次；`--include-bbox=false` 可关闭启动时已开启的 bbox。隐藏输出不改变 PDF 内部抽取、段落合并及裁图所需的几何计算。`include_image` 独立控制搜索附图，显式 `view` 仍渲染图片；与 bbox 是否输出无关。PDF图片默认作为路径返回；外部需要图片字节时使用 `--inline-images`。

模型工具的裁剪坐标统一放在 `region:{left,top,right,bottom}` 对象中，不展示同义的顶层裁剪字段；旧 CLI 的 `--left/--top/--right/--bottom` 和旧 JSON flat 参数仍兼容。两种坐标写法不能同时传入。

CLI `--output-version 1|2` 选择启动默认输出版本，例如：

```bash
build/compare-figure document-search --query Dpolyline3d --output-version 2 --include-bbox=false
build/compare-figure document-search --mode read --doc doc/luadraw-doc-en.pdf --page 14 --include-bbox=true
build/compare-figure document-search --query Dpolyline3d --output-version 1
```

## JSON 与 JSONL

单个请求支持裸工具参数：

```json
{"mode":"search","query":"Dpolyline3d","limit":3,"snippet_chars":1200,"max_chars":6000}
```

也支持完整 envelope，用于每次请求指定缓存、资源和图片输出：

```json
{
  "operation": "docs",
  "cache_dir": "work/shared-doc-cache",
  "options": {"doc_output":{"version":2,"include_bbox":false}},
  "arguments": {"mode":"search","query":"Dpolyline3d","include_image":false,"include_bbox":true}
}
```

```bash
build/compare-figure luadraw-doc --request examples/luadraw-doc.json.example
build/compare-figure luadraw-doc --stdin --cache-dir work/shared-doc-cache
build/compare-figure luadraw-doc --jsonl --cache-dir work/shared-doc-cache
```

将单个JSON写给 `--stdin`；JSONL每行一个JSON，stdout每行一个JSON响应，共用长驻索引实例。`--request`、`--stdin`、`--jsonl`互斥；专用入口只执行docs，不允许通过JSON切换到run/compile。JSON字段可覆盖CLI默认值，未指定字段沿用显式CLI参数。envelope 的 `options.doc_output` 是启动默认；工具 `arguments.output_version`（整数 1 或 2）和 `arguments.include_bbox`（可选布尔 true/false）优先于启动值，省略才继承。因此上例使用 v2，并为本次请求开启 bbox。裸工具参数也支持这两个覆盖字段。旧接口 `tools --request FILE`、`tools --jsonl` 继续支持 envelope。

app 外层协议 `version` 仍为 `compare-figure-tools/1`，新增整数 `output_version` 表明本次有效文档输出版本，不把外层版本改为 `/2`。v2 响应保留 `ok`、`results`、`images`、`out_dir` 及分页/warnings 等机器字段，正文只在各 `results[].text` 中出现，省略顶层 `text`、`cache` 和 `provenance`；PDF 结果保留 source/doc/page，bbox 仅显式开启时输出 `bbox_pct`，不输出点坐标或页面尺寸。v1 保留原有 `text/results/cache/provenance` 字段供旧消费者使用；模型文字仍采用上述紧凑格式。普通文字请求的images为空；显式view返回路径、MIME和图片尺寸，inline_images请求另含base64。单次错误退出2；JSONL坏行单独返回错误、继续下一行，结束时有错误则退出1。调用方必须读取ok，不能把错误JSON当文档正文。

外部 Python 程序可以通过子进程调用，生产工具本身不依赖Python：

```python
import json
import subprocess

request = {"mode": "search", "query": "Dpolyline3d", "limit": 3}
completed = subprocess.run(
    ["/absolute/skill/build/compare-figure", "luadraw-doc", "--stdin",
     "--cache-dir", "/absolute/writable/shared-doc-cache"],
    input=json.dumps(request), text=True, capture_output=True, check=True,
)
reply = json.loads(completed.stdout)
if not reply["ok"]:
    raise RuntimeError(reply)
for result in reply.get("results", []):
    print(result["text"])
```

## Go 直接调用

当前 Go module 为 `compare-figure`，源码在 `go/`。在同一模块使用公开包，或外部模块通过本地 `replace compare-figure => /absolute/skill/go` 接入；没有宣称已发布公共Go模块版本。直接接口无需CLI子进程：

```go
import (
    "context"
    "compare-figure/luadraw"
)

// libraryRoot 指向 vendor/luadraw-3.7/luadraw，cacheRoot为可写目录。
docs, err := luadraw.NewDocs(libraryRoot, cacheRoot)
if err != nil { return err }
text, imagePath, err := docs.Execute(context.Background(), map[string]any{
    "mode": "search", "query": "Dpolyline3d", "limit": 3,
}, outputRoot)
// 调用方处理err，并使用text/imagePath。
_ = text
_ = imagePath
return err
```

`documents.New`（以及 `luadraw.NewDocs` 宿主兼容入口）的启动默认是 v2；直接 `documents.NewConfig` 保留核心输出配置及未指定时的 v1 默认。`Execute` 继续返回模型文字和图片路径，机器消费者直接调用 `ExecuteResponse` 时按其 `version`（`doc-library/1` 或 `doc-library/2`）处理结构化响应；只有 app 封装层保持 `version: "compare-figure-tools/1"` 并另加整数 `output_version`。保留同一个Docs实例可复用内存索引；`Definition()`返回模型工具schema，`Execute()`处理该schema的参数。`FrequentSnippet(ctx, limit, lineChars, maxChars)`仅获取被动摘要，不增加计数；显式Execute frequent会计数。

## 缓存位置与频次规则

- 默认位置为Skill根的 `build/doc-cache`，CLI显式 `--cache-dir` / JSON `cache_dir` 优先；否则 `COMPARE_FIGURE_CACHE` 可覆盖默认目录。Go `NewDocs` 请显式传cacheRoot以取得相同位置策略。
- 相同cache目录可跨进程/会话共享；JSONL同一进程还复用内存对象。并行图片请求应各自指定out目录，避免默认预览覆盖；仅文字查询可以共用缓存。
- 索引和频次采用 doc-library 的根／配置命名空间与逐文件 SHA 校验；同大小、同 mtime 的内容改写也会失效。旧 v4 缓存及 entry_id 不直接迁移，首次查询会重建。
- 高频最多256条，每条正文最多1024字符。主动 search/read/view/grep/frequent 只为实际返回的文档正文计数；list/find 目录导航及被动摘要不计数。
- 默认frequent为8条、每行256字符、总正文预算4096；需要更长条目可传 `line_chars:1024`。支持短 `entry_id` 或排名范围选择。排名累计次数，无时间衰减。
- 当前Agent原创绘图没有自动runner开场注入：请主动先调用frequent；自动runner的新任务开场摘要默认8行、每行1024字符、总预算4096字符，显式更小预算仍生效，配置docs.frequent可关闭，语义不变。
- 多进程写入由锁和原子替换保护。缓存属于可重建本地状态，不进Git、不附在源码Skill包中；不同项目想隔离历史，指定不同cache目录即可。

所有参数详见 `luadraw-doc --help`。本工具面向本地CLI/JSONL/Go调用，未新增HTTP服务或模型SDK依赖。
