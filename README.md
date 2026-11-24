# compare-figure

**用 LuaDraw 绘制、重绘和检查高质量矢量插图，交付可编辑的 LaTeX 源码与 PDF。**

可以根据参考图片还原图形，也可以从文字描述创作数学图、几何图、函数图和示意图；支持结合 Markdown 的插图上下文，或为 LaTeX 文稿绘制可嵌入的图稿。内置 LuaDraw 官方文档检索、编译、图片比对与本地会话查看器，帮助完成“查文档 → 绘图 → 编译 → 看图 → 修正”的过程。

## 快速安装

复制下面这段话给你的 Agent：

```text
请安装并配置 compare-figure Skill：
https://github.com/LingNc/compare-figure

请先阅读仓库 README.md 和 SKILL.md，再安装到当前工具支持的 Skill 目录。
如果仓库已有正式 Release，优先获取源码 Skill ZIP 和对应 SHA256，校验后解压；
否则克隆仓库并使用根目录 SKILL.md。
按说明检查 Go 1.25+、Lua/TeX 和 Poppler，首次构建本机程序，
再做不调用付费模型的文档检索与编译检查，告诉我安装位置和可用功能。
默认使用当前 Agent 绘图；仅在我要求自动 runner 时配置额外模型。
```

安装完成后，直接描述要画什么，或提供参考图、Markdown／LaTeX 文稿和图形要求即可。正式发布包为源码包，首次构建和依赖检查由上述安装步骤完成，后续复用。

## 怎么使用

| 想完成的事情 | 可以这样描述 |
|---|---|
| 按图片重绘 | “把这张图用 LuaDraw 重绘，保留标注、比例和线型，给我可编辑 TeX 和 PDF。” |
| 从描述创作 | “画一个椭球与平面相交的三维示意图，标注坐标轴和截面，适合黑白讲义。” |
| 处理 Markdown 插图 | “重绘这份 Markdown 中的指定插图，结合图前后的题意和符号，给出图稿与替换建议。” |
| 为 LaTeX 文稿配图 | “为这段 LaTeX 题目绘制函数与阴影区域图，字体和数学标注与正文一致。” |
| 查询 LuaDraw 用法 | “先检索官方手册和源码示例，确认曲面、箭头和标注的方法，再绘图。” |

主要有两种工作方式：

- **当前 Agent 绘图**：直接查文档、写 LuaDraw、编译、看图和修正，无需额外模型密钥；既可重绘，也可原创。
- **自动绘图**：提供支持图片与工具调用的模型接入，让 runner 自动迭代参考图重绘，保留会话并支持中断续跑。

参考图重绘支持图片及可选的实际尺寸、Markdown 上下文，并提供像素叠图和尺寸偏差检查。原创图按描述检查结构、标注与样式，不需要参考图片或比对分数。Markdown／LaTeX 文稿可作为绘图上下文和图稿嵌入目标；当前不是一键转换整篇文稿的工具。

## 能得到什么

- **可编辑图稿**：自动 runner 交付内嵌 LuaDraw 代码的独立 TeX、原 Lua、PDF 和预览；当前 Agent 绘图也按此形式整理交付。
- **文档辅助**：检索官方手册、查看 PDF 页／区域，或读取 TeX／Lua 文件的指定行；共享高频缓存可跨任务复用，也可供其他程序独立调用。
- **重绘检查**：结合叠图、物理尺寸和实际图像检查还原效果；分数不能代替内容核对。
- **会话回看**：用 Chrome／Edge 打开 [viewer.html](viewer.html)，选择会话目录，查看工具过程和图片。只读本地目录，按需加载，不上传；手动刷新读取新内容。

## 详细说明

| 内容 | 入口 |
|---|---|
| Skill 行为与完整操作流程 | [SKILL.md](SKILL.md) |
| CLI、尺寸、Markdown 输入、配置、续跑与工具 | [详细使用说明](docs/usage.md) |
| 独立文档工具：CLI／JSON／JSONL／Go 接口、缓存 | [LuaDraw 文档检索](docs/luadraw-doc.md) |
| 原生 Go 接口、构建和资源导出 | [Go 运行说明](docs/go-native.md) |
| CI 检查、版本标签与源码 Skill 发布 | [发布说明](docs/releasing.md) |
| 已验证范围、历史变更和后续计划 | [开发记录](docs/notes.md) |

## 构建与配置

仓库和 Skill 包均只附生产源码，不带预构建程序。首次需要 [Go 1.25+](https://go.dev/dl/)（[安装说明](https://go.dev/doc/install)）；绘图需要 LuaLaTeX，或 Lua/texlua 配合 XeLaTeX/pdfLaTeX，文档与 PDF 操作需要 Poppler。正式程序不依赖 Python。

```bash
git clone https://github.com/LingNc/compare-figure.git
cd compare-figure
go -C go build -o ../build/compare-figure ./cmd/compare-figure
build/compare-figure environment
build/compare-figure luadraw-doc --query Dpolyline3d
```

Windows 构建输出名改为 `../build/compare-figure.exe`。编译结果的实际路径以 JSON 响应为准。

自动 runner 的配置从 [注释示例](examples/runner.yaml.example) 复制，密钥建议通过环境变量传入。真实配置、缓存和运行数据不进入源码 Skill 包。当前 Agent 绘图不需要填写这份模型配置。

```bash
cp examples/runner.yaml.example runner.yaml
# 配置模型后，按参考图启动自动绘图
build/compare-figure run --original reference.png --out runs/figure-001 --config runner.yaml
# 原创或重绘图稿均可单独编译
build/compare-figure compile --source figure.tex --out work/compiled
```

## 开发与发布

```bash
go -C go test ./...
go -C go vet ./...
node scripts/test_viewer.mjs
./scripts/export_skill.sh
```

导出版本取自 [VERSION](VERSION)，生成源码 Skill ZIP、SHA256 和逐文件清单。GitHub Actions 配置在代码更新时测试并验证源码包，匹配版本的 `vX.Y.Z` 标签可触发 Release 发布；仅推送代码不会自动发布正式版本，详见 [发布说明](docs/releasing.md)。首次远端运行结果以 Actions 记录为准。

项目采用 [MIT 许可](LICENSE)，固定 LuaDraw 3.7 资源保留原许可。依赖和提示词来源见 [固定依赖](vendor/dependencies.lock.json) 与 [来源清单](docs/upstream.json)。
