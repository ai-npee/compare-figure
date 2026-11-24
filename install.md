# 安装 compare-figure

本流程适用于源码仓库和源码 Skill 包。默认由当前 Agent 绘图，无需额外模型密钥。首次安装会构建本机程序；后续直接复用。

## 1. 获取并放置 Skill

将整个目录放到当前工具支持的 Skill 目录或项目目录中，保持根目录的 `SKILL.md`、`go/`、`vendor/` 等文件相对位置不变。不同 Agent 的技能目录约定不同，按所用工具注册根目录的 `SKILL.md`，不要只复制该文件。

- 有正式 [Release](https://github.com/ai-npee/compare-figure/releases) 时：下载 `compare-figure-skill-vX.Y.Z.zip` 和同名 `.zip.sha256`，校验后解压。不要误用 GitHub 自动生成的 Source code ZIP 作为已校验的 Skill 发布包。
- 没有正式 Release 时，克隆仓库：

```bash
git clone https://github.com/ai-npee/compare-figure.git
cd compare-figure
```

Linux 校验下载包：`sha256sum -c compare-figure-skill-vX.Y.Z.zip.sha256`。macOS 使用 `shasum -a 256 -c ...`；Windows 用 `Get-FileHash 包路径 -Algorithm SHA256` 与校验文件中的值比较。

以下命令均在解压后的 Skill 根目录或仓库根目录执行。

## 2. 检查依赖

| 依赖 | 用途 |
|---|---|
| [Go 1.25+](https://go.dev/dl/)（[安装步骤](https://go.dev/doc/install)） | 首次构建程序；首次可能下载 Go 模块 |
| TeX Live 或其他兼容 TeX 环境 | 生成 PDF，优先 LuaLaTeX；也支持 Lua/texlua → TikZ → XeLaTeX/pdfLaTeX |
| Poppler：`pdftotext`、`pdfinfo`、`pdftoppm` | 文档索引、PDF 读页和图片预览 |

LuaDraw 3.7 已随包提供，不需要另外下载。正式运行不需要 Python。安装依赖时复用可用环境，不改写现有项目的配置或密钥。

Ubuntu／Debian 可安装下列依赖；Go 另按官方说明安装到 1.25 或更高版本：

```bash
sudo apt-get update
sudo apt-get install lua5.4 poppler-utils texlive-luatex texlive-xetex \
  texlive-latex-extra texlive-fonts-recommended texlive-lang-chinese
```

Windows／macOS 安装 Go、TeX Live 和 Poppler，并把相关程序加入 `PATH`。先用 `go version` 确认版本；仅能启动 TeX 命令不代表完整绘图依赖已经可用，下一步继续探测。

## 3. 构建与验证

```bash
go -C go build -o ../build/compare-figure ./cmd/compare-figure
build/compare-figure environment
build/compare-figure luadraw-doc --mode list --doc files --limit 3
build/compare-figure luadraw-doc --query Dpolyline3d --limit 2
```

Windows PowerShell：

```powershell
go -C go build -o ../build/compare-figure.exe ./cmd/compare-figure
.\build\compare-figure.exe environment
.\build\compare-figure.exe luadraw-doc --query Dpolyline3d --limit 2
```

查看 JSON 响应和环境报告，确认文档检索及可用编译路线；缺少依赖时报告具体缺项，不将部分可用称为安装全部完成。文档初始化需要 Poppler；缓存默认在 Skill 根的 `build/doc-cache`，也可用 `--cache-dir` 指定可写目录。

安装验证不调用付费模型。需要验证具体图稿时再执行 `compile --source figure.tex --out work/compiled`，并检查返回的 PDF 和预览。没有图稿时无需为了安装启动自动重绘。

## 4. 开始使用

让当前 Agent 阅读根目录的 [SKILL.md](SKILL.md)，然后提供参考图或说明想画什么。操作其他项目时使用可执行程序的绝对路径；资源可自动从 Skill 根发现，必要时指定 `--resource-root`。

- 当前 Agent 绘图：查文档、编写 LuaDraw、编译、查看并修正，不需要额外模型配置。
- 自动 runner：仅在需要时从 [配置示例](examples/runner.yaml.example) 创建本地配置并接入模型，密钥优先使用环境变量。

更新时下载并校验新版本，或在无本地冲突的仓库中拉取更新，再重新构建。保留自己的配置和图稿，缓存不需要随安装包复制。完整用法见 [SKILL.md](SKILL.md)、[独立文档工具](docs/luadraw-doc.md) 和 [仓库详细说明](https://github.com/ai-npee/compare-figure/blob/main/docs/usage.md)。
