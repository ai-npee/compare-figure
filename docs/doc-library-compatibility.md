# doc-library 接入与 LuaDraw 兼容矩阵

compare-figure 的文档检索核心现直接引用 [LingNc/doc-library](https://github.com/LingNc/doc-library)，固定提交 `a705bca4420cd2126b6072a1c7b6450f2b9cc84c`，Go pseudo-version `v0.0.0-20261124124631-a705bca4420c`。上游当前声明 `module doc-library`，因此主模块使用远程 module replacement：

```go
require doc-library v0.0.0
replace doc-library => github.com/LingNc/doc-library v0.0.0-20261124124631-a705bca4420c
```

不存在指向相邻工作目录的 replace，也没有保留第三方核心源码副本。Go 从上游下载的模块由校验和锁定；首次构建需要已缓存模块或能访问 Go/GitHub 下载通道。模块缓存完整时可以 `GOPROXY=off` 构建。源码 Skill 的可搬移性不依赖开发机的 doc-library checkout，离线首次构建仍要求目标环境预先备有这三个依赖模块的缓存。

[通用适配器](../go/documents/documents.go)只负责选择资源、旧参数转换、文本展示、裁图像素适配和被动摘要预算；索引、PDF 抽取、正文段落、去重合并、路径限制、缓存失效和频次事务都由独立库执行。[LuaDraw 入口](../go/luadraw/docs.go)是相同实现的薄代理。

## 接口与模式

```go
// rootResource 为 Skill 根；也兼容原 LuaDraw vendor 根。
d, err := documents.New(rootResource, cacheDir, "mixed")
text, imagePath, err := d.Execute(ctx, args, outDir)
response, err := d.ExecuteResponse(ctx, args, outDir)
summary, err := d.FrequentSnippet(ctx, 8, 256, 2048)

// Sources、filters、cache、limits、PDF 可用 doc-library 原生配置。
d, err = documents.NewConfig(documents.Config{
    Sources: []documents.Source{{ID: "custom", Root: "/local/manuals"}},
    CacheDir: cacheDir,
})
```

| 后端 | 默认附带文档源 | 说明 |
| --- | --- | --- |
| `luadraw`（默认） | `source=luadraw` | 固定 LuaDraw vendor 根全部可读资源；二进制文件不作为正文索引 |
| `mixed` | `luadraw`、`asymptote` | 可跨源搜索；重名文档 read/view 必须指定 source |
| `asymptote` | `source=asymptote` | 从 [上游资源清单](asymptote-upstream.json)的 `resource_root` 发现；没有声明时要求唯一 `vendor/asymptote-*` |
| `latex` | 无默认专用库 | 返回成功响应及明确说明，不加载 LuaDraw 参考；调用方可用 NewConfig 配置自己的 LaTeX 文档 |

通用工具名为 `document_search`，旧 `luadraw.NewDocs(rootVendor, cacheDir)`、`Definition()` 的 `luadraw_doc` 名称、`Execute` 三返回与四参数 `FrequentSnippet` 都保留。结构化 ExecuteResponse 增加 `results`、`cache`、`warnings` 和固定上游 `provenance`，仍提供 `text` 与 `images`。

## 能力对照

以下比较原 LuaDraw 文档模块与当前适配器。七类主要查询能力都覆盖；正文、图片、来源定位、文件边界和主动/被动频次已经实测。这里的“功能覆盖”不代表旧文本每个字符、排序、统计元数据和缓存文件布局保持不变。

| 项目 | 原行为 | 当前行为及差异 |
| --- | --- | --- |
| search | TeX/Lua 段落与手册 PDF；默认文字结果 | 保留，另支持多源、kinds/extensions、any/all/phrase、大小写和正则过滤；完整根搜索范围稍扩大 |
| 段落/合并/去重 | 源码连续段落、PDF 同页相邻块；不合并跨列 | 独立库承接原算法，PDF 使用 Poppler flow/block/line 层级；跨页、跨源不会混成一块 |
| 精确符号 | 通过词项/短语加分突出 API | 支持；真实 `Dpolyline3d` 查询可获得完整签名和 draw_options，不承诺首条排名逐项不变 |
| read 文本 | doc 或 query 文件路径，1-based 行号，最多400行 | 保留；明确 start_line/end_line，默认400行；显式配置可调整配额；仍输出行号 |
| read PDF | doc 或 query，加 page 或 `#page=N` | 保留，冲突的两个页选择器报错；1-based 物理页，行范围指该页规范段落块 |
| view PDF | 整页或 left/top/right/bottom 百分比区域，zoom 为图片宽 | 保留；旧平铺百分比转为库 region，也接受 region；显式区域预放大整页渲染后归一到请求宽度，保持普通裁图清晰度 |
| 窄区域极端放大 | 最终zoom≤5000，内部整页宽最高12000 | 上游整页zoom≤5000、DPI≤600。极窄区域可能比旧内部12000上限的极端裁图清晰度低；没有绕开库另写PDF渲染器 |
| search 附图 | 默认不附图，include_image显式请求 | 保留；只对实际PDF结果附图，显式view始终渲染 |
| PDF 定位 | page、bbox_pts、bbox_pct、页面尺寸 | 保留于text，并额外在results中结构化给出source/doc/page/bbox/bbox_pct/page_width/page_height |
| list | 根目录/子目录、默认一层，可recursive | 保留整个LuaDraw根的导航；recursive列文件，不再额外列出已递归经过的每个目录行 |
| find | 路径子串/文件名glob、scope | 保留，按来源和路径稳定排序；支持库的递归glob |
| grep | 字面/RE2正则，doc范围，默认区分大小写 | 保留；适配器为未显式设置case_sensitive的grep补true，保持旧行为；显式false可不区分大小写。直接产生真实命中行并计入正文频次，search默认大小写行为不变 |
| 分页 | limit、offset | 保留；结构化next_offset及warning说明可能还有结果，不承诺总结果数 |
| 片段预算 | snippet_chars与max_chars；旧search将位置header算入正文预算 | 保留正文预算。当前max_chars约束各result.text的总和；text中的来源header、行号、cache说明和warning在预算外，不能把整段tool text当作严格max_chars上限 |
| 被动开场摘要 | 8行/2048字符上限，不计频 | 保留。完整摘要包括位置、ID、次数也≤2048字符；不主动渲染、不触发PDF抽取 |
| 主动frequent | 排名区间、doc/行范围、entry_id前缀 | 保留；主动返回项计频，passive=true不计频；旧负limit表示默认8条的调用也兼容 |
| 正文频次 | read/search/view及主动frequent，稳定段落ID | 保留；grep真实命中也作为条目。多源ID包含source，文件内容变更后只隐藏该文件旧条目，不清空整个库的未变文件 |
| 导航频次 | list/find把整个目录输出保存成伪段落 | 有意不保留。上游只累计真实文档段落，不把目录列表和空匹配混入高频知识 |
| 路径安全 | 相对路径、拒绝..和绝对路径、符号链接不能越根 | 保留，使用Go rooted filesystem限制；NewConfig的include/exclude/extensions对read同样生效 |
| 错路径建议 | 计算邻近实际文件名，建议list/find | 保留真实路径提示；通过库list给出请求目录的可用路径，不再维护第二套模糊距离索引 |
| 初始化依赖 | 构造时统一要求三项Poppler工具 | 改为按需：text-only无需Poppler；PDF读取需pdftotext，显式图片需pdftoppm。不执行文档中的代码 |
| 缓存 | 全库v4索引及全局频次JSON，主要依赖mtime/size | 改为根/配置namespace、逐文件内容SHA和跨进程锁；同size、同mtime内容替换仍失效；用户传入的cacheDir仍生效 |
| 缓存格式/旧ID | 固定luadraw-doc-index-v4.json与frequency文件、旧ID | 不迁移旧缓存字节或旧entry_id。缓存可重建；重建后用新的返回ID。不同来源/过滤配置有隔离namespace |
| 文本元数据 | LuaDraw版本、schema、mtime、创建时间；行数/页数总量 | 保留index_sha256说明并增加namespace/cache counters；不再伪造上游未返回的总行数/总页数和旧schema4元数据，tests改验真实位置与结果 |
| Go/JSON工具契约 | text+可选图片；独立CLI支持JSON/JSONL | 旧Go三返回保留；新增结构化响应入口由CLI接线。旧CLI别名和JSONL逐行隔离由app层维护 |
| PDF到TeX源行 | 不具备精确映射 | 仍不具备；PDF物理页/bbox、Texinfo/TeX文本行数不代表SyncTeX反向映射 |

原模块没有“手册标题”“texdoc”等虚构文件别名。受支持的别名是 read 的 query/doc 互换、真实PDF的 `#page=N`、独立page以及旧工具入口名；这些保留。不存在文件不会被默默改写成另一个文件。

## 本轮验证

[通用API回归](https://github.com/ai-npee/compare-figure/blob/main/go/documents/documents_test.go)使用真实固定LuaDraw资源和Asymptote3.16资源，验证普通短语、完整API签名、PDF坐标、显式搜索附图、TeX/Lua源行、list/find/grep、PDF两种页选择写法、页冲突拒绝、百分比view、active/passive frequent、重新创建服务后的暖缓存、四模式和mixed来源过滤。Asy另实读Texinfo原始文本、检索.asy源例并读取官方PDF，不增加Texinfo语义解析。

[导航回归](https://github.com/ai-npee/compare-figure/blob/main/go/luadraw/doc_navigation_test.go)验证规范段落、分页、snippet预算、RE2与字面grep（`Abc`/`abc`默认精确大小写、显式false同时命中，通用和旧入口均覆盖，search行为不变）、400行限制、缺失路径提示、绝对路径/父路径/Windows路径/符号链接边界。[频次回归](https://github.com/ai-npee/compare-figure/blob/main/go/luadraw/doc_frequency_test.go)用4个实例各并发读取3次，验证次数精确累加、不同读取窗口共享规范段落ID、rank/entry_id前缀选择、被动摘要不改磁盘、内容SHA失效及非法UTF-8拒绝。

离线专项测试已通过。完整LuaDraw根扫描观察到240个加载候选，其中239个可抽取文件暖态全部命中、0 miss；另外一个非正文候选被安全跳过，这个统计不是声称245个固定vendor文件都要建立正文索引。真实英文手册物理第14页8/10/90/80百分比区域生成850×1025图片并实际查看，保留文本与代码格式。缓存和预览证据只放忽略的build区域。上游库和LuaDraw245资源未修改；没有调用付费模型。
