# 绘图库模式

工作方式（自动 runner／当前 Agent）与绘图库策略相互独立。两种工作方式均可选择以下四种策略，默认 `luadraw`。

| 值 | 允许的绘图源码 | 自动检索资料 |
|---|---|---|
| `luadraw` | `.lua` 或含真实 LuaDraw 环境的 `.tex` | 固定 LuaDraw 3.7 文档与源码 |
| `latex` | 常规 `.tex`，如 TikZ、PGFPlots、picture 等已安装 TeX 包 | 默认不注入两套专用库的资料 |
| `asymptote` | `.asy` 或包含 `asy` 环境的 `.tex` | CTAN Asymptote 3.16 手册、Texinfo 文本与 `.asy` 示例 |
| `mixed` | 模型按图选择 LuaDraw、Asymptote 或常规 LaTeX | 同时提供 LuaDraw 与 Asymptote |

混合策略允许选择适合的后端，不要求同一个源文件强行同时使用两套语言。模式限制在编译入口执行，而非仅靠提示词约束。`--engine` 仍指 TeX 引擎，与绘图库不同。

`--engine auto` 会优先遵循 Asy 程序中独立语句行的显式 `settings.tex="xelatex"`（同样支持 `lualatex`／`pdflatex`），使标签前导与外层可编辑 TeX 使用一致的引擎；例如 `xeCJK` 必须由 XeLaTeX 处理。未指定源码引擎时沿用可用工具优先级；显式 `--engine` 优先于自动推断。

## 设置

自动 runner 的 YAML：

```yaml
drawing:
  mode: mixed
```

CLI：

```bash
compare-figure run --original reference.png --out runs/figure --config runner.yaml --drawing-mode mixed
compare-figure compile --source figure.tex --drawing-mode latex --out work/native
compare-figure compile --source figure.asy --drawing-mode asymptote --out work/asy
compare-figure environment --drawing-mode asymptote
```

JSON 请求使用顶层 `drawing_mode`。同一任务的编译、比较和提交沿用相同策略；改变策略时使用新工作目录，避免旧接受态直接返回另一策略的成品。不指定策略的旧调用继续默认 LuaDraw；已有保存任务的续跑规则见接口说明。

## 文档工具

`document_search` 是模型工具，独立命令为 `document-search`。底层直接使用 [doc-library](https://github.com/LingNc/doc-library) 的固定上游 Go 模块，索引、段落、位置、频次和缓存不再在本项目维护另一份实现。

```bash
compare-figure document-search --drawing-mode mixed --doc-source asymptote --query projection
compare-figure document-search --drawing-mode mixed --doc-source luadraw --query graph3d
compare-figure document-search --drawing-mode asymptote --mode read --doc doc/asymptote.texi --start-line 1 --end-line 40
```

模型工具参数以 `source` 选择文档库。`search/read/view/list/find/grep/frequent` 继续可用，旧 `luadraw_doc`、`luadraw-doc` 和 docs JSON 请求保留。Texinfo 在此作为普通 UTF-8 文本检索和按行读取，不是 Texinfo 语义解析器。功能与兼容边界见 [差异矩阵](doc-library-compatibility.md)。

LuaDraw／Asymptote 文档版本固定，不替代宿主编译程序；不同版本新增 API 应以实际 `environment` 和编译结果确认。原生 LaTeX 策略不假称附带所有 TeX 包的手册。

## 编译与交付

- LuaDraw 保持 LuaLaTeX 及独立 Lua／texlua → TikZ → XeLaTeX／pdfLaTeX 回退。
- 原生 LaTeX 编译完整文档，也可将绘图片段放入自动独立页包装中。可选用已安装的包；完整文档自行声明导言区。
- Asymptote 调用可用的 `asy` 生成静态 PDF。嵌入式 Asy TeX 采用 TeX → 显式编译生成的 `.asy` → TeX 的流程，不依赖自动 shell escape。

交付保留实际接受的源码、独立可编辑 `submitted.tex` 和 PDF／预览，并记录 `drawing_mode`、实际 `source_language`、`backend`、`engine`。Asymptote 的独立 TeX 内嵌 Asy 程序，需要额外 Asymptote 编译步骤；使用本工具的 `compile --drawing-mode asymptote` 自动处理，单独运行一次普通 TeX 命令不能代替整个流程。原始 `.asy` 也会保留。

无参考图可直接创作、编译和看图，不进入比较／提交评分。重绘仍采用原比较与接受门槛，换绘图库不改变分数或回执。交互式 WebGL／PRC 不作为当前交付目标；3D 以可预览、可比较的静态 PDF 为准。

Asymptote 安装与运行依赖见 [安装流程](../install.md)，固定资料的版本、下载来源、SHA256 与许可见 [出处清单](asymptote-upstream.json)。
