# 文档检索工具与 LuaDraw 兼容入口

文档索引、段落、PDF 抽取与频次现由独立 [doc-library](https://github.com/LingNc/doc-library) 固定上游依赖提供。`document_search`／CLI `document-search` 是通用入口，旧 `luadraw_doc`、`luadraw-doc` 和 Go `luadraw.NewDocs` 保留。默认仍检索 LuaDraw；`--drawing-mode asymptote` 检索 Asymptote，`--drawing-mode mixed` 同时提供两者，`--doc-source luadraw|asymptote` 可筛选来源。下方旧命令示例仍适用，也可将命令名换为 `document-search`。

该工具不要求参考图、Markdown 书稿、自动绘图会话或模型密钥。新增 Go `documents.New(resourceRoot,cacheDir,mode)` 与 `NewConfig` 可配置多个文档根；结构化 `ExecuteResponse` 包含 results/cache/provenance。模式配置见 [绘图库说明](drawing-modes.md)，完整功能对照、输出和缓存差异见 [兼容矩阵](doc-library-compatibility.md)。默认资料不是任意 TeX 包的手册全集。

## 原创图工作方式

1. 用户说明想画的对象、标签、布局和用途，当前 Agent 分析构造。
2. 先调用 `frequent` 复用高频知识；缓存为空则继续 `search`。用 `list/find/grep` 定位实际文档和依赖，用 `read` 精读 API/示例。
3. 需要理解官方图示时才 `view` 指定 PDF 页或区域；普通查询默认不附图。
4. Agent 写 LuaDraw `.lua` 或内嵌 `luadraw` 环境的 `.tex`，调用 `compile`，查看 PDF 和预览，按用户要求修正。
5. 交付可编辑源码和 PDF。没有参考图时不报告比对分数，也不需要进入参考图 `submit` 接受流程。

## 构建和依赖

Skill 只附 Go 源码。安装 Go 1.25+（[下载](https://go.dev/dl/)、[安装](https://go.dev/doc/install)），在包根目录运行：

```bash
go -C go build -o ../build/compare-figure ./cmd/compare-figure
```

Windows 输出名使用 `../build/compare-figure.exe`。首次构建会下载固定 doc-library 等 Go 模块，之后运行不需要旁边的独立仓库。文本源码查询不需 Poppler；PDF 文字检索／读页需 `pdftotext`，显式图片预览需 `pdftoppm`。文档读取不要求 LuaLaTeX、Asymptote 或模型密钥。绘图编译按所选策略另需相应程序；LuaDraw 保留原两路线。

从其他项目调用时使用可执行程序的绝对路径。资源自动从 Skill 根定位，或用 `--resource-root` 指定 Skill 根；`--luadraw-root` 指定固定库根。文档内容只能在库根内访问，不开放任意宿主文件。

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

源码按行读取最多400行，PDF按页/段落块定位。搜索提供文档、行范围或PDF页码/bbox，`snippet_chars`控制每个片段，`max_chars`控制正文总预算（工具元信息另计）。`view`百分比裁切与搜索bbox点坐标不同，按返回的 `bbox_pct` 可选区域。PDF图片默认作为路径返回；外部需要图片字节时使用 `--inline-images`。

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
  "arguments": {"mode":"search","query":"Dpolyline3d","include_image":false}
}
```

```bash
build/compare-figure luadraw-doc --request examples/luadraw-doc.json.example
build/compare-figure luadraw-doc --stdin --cache-dir work/shared-doc-cache
build/compare-figure luadraw-doc --jsonl --cache-dir work/shared-doc-cache
```

将单个JSON写给 `--stdin`；JSONL每行一个JSON，stdout每行一个JSON响应，共用长驻索引实例。`--request`、`--stdin`、`--jsonl`互斥；专用入口只执行docs，不允许通过JSON切换到run/compile。JSON字段可覆盖CLI默认值，未指定字段沿用显式CLI参数。旧接口 `tools --request FILE`、`tools --jsonl` 继续支持 envelope。

响应主要字段为 `ok`、`text`、`images`、`out_dir` 和 `version`。普通文字请求的images为空；显式view返回路径、MIME和尺寸，inline_images请求另含base64。单次错误退出2；JSONL坏行单独返回错误、继续下一行，结束时有错误则退出1。调用方必须读取ok，不能把错误JSON当文档正文。

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
print(reply["text"])
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

保留同一个Docs实例可复用内存索引；`Definition()`返回模型工具schema，`Execute()`处理该schema的参数。`FrequentSnippet(ctx, limit, lineChars, maxChars)`仅获取被动摘要，不增加计数；显式Execute frequent会计数。

## 缓存位置与频次规则

- 默认位置为Skill根的 `build/doc-cache`，CLI显式 `--cache-dir` / JSON `cache_dir` 优先；否则 `COMPARE_FIGURE_CACHE` 可覆盖默认目录。Go `NewDocs` 请显式传cacheRoot以取得相同位置策略。
- 相同cache目录可跨进程/会话共享；JSONL同一进程还复用内存对象。并行图片请求应各自指定out目录，避免默认预览覆盖；仅文字查询可以共用缓存。
- 索引和频次采用 doc-library 的根／配置命名空间与逐文件 SHA 校验；同大小、同 mtime 的内容改写也会失效。旧 v4 缓存及 entry_id 不直接迁移，首次查询会重建。
- 高频最多256条，每条正文最多2048字符。主动 search/read/view/grep/frequent 只为实际返回的文档正文计数；list/find 目录导航及被动摘要不计数。
- 默认frequent为8条、每行256字符、总正文预算4096；支持短 `entry_id` 或排名范围选择。排名累计次数，无时间衰减。
- 当前Agent原创绘图没有自动runner开场注入：请主动先调用frequent；自动runner的新任务开场摘要默认8行/2048字符，配置docs.frequent可关闭，语义不变。
- 多进程写入由锁和原子替换保护。缓存属于可重建本地状态，不进Git、不附在源码Skill包中；不同项目想隔离历史，指定不同cache目录即可。

所有参数详见 `luadraw-doc --help`。本工具面向本地CLI/JSONL/Go调用，未新增HTTP服务或模型SDK依赖。
