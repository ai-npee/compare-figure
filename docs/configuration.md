# Runner 配置

本页说明自动 runner（`run`、`auto` 以及 `tools` 的 `operation: "run"`）如何选择配置。配置只影响 runner；`compile`、`docs`、`compare` 和独立 `tool` 操作不会因为当前目录存在 `runner.yaml` 就读取它。`run --environment` 与 `run` 使用同一套发现规则。

## 先发现，再启动

Agent 启动自动绘图前按下面顺序查找，**只选第一个，不合并多个文件**：

1. 显式 `--config PATH`，或 JSON runner 请求中的 `config`。
2. `COMPARE_FIGURE_CONFIG` 环境变量指定的路径。
3. 当前进程工作目录中的 `runner.yaml`、`runner.yml`、`runner.json`，按此顺序尝试。
4. Skill/resource root 中的同名文件，仍按 `runner.yaml`、`runner.yml`、`runner.json` 顺序尝试；如果 root 就是当前工作目录，会去重。

显式路径和 `COMPARE_FIGURE_CONFIG` 指向不存在、非普通文件或无法解析的文件时，命令直接报错，**不会回退**到其他位置。自动发现到的文件如果解析失败也直接报错。文件中的相对路径按进程当前工作目录解释；程序不会按 YAML 所在目录改写路径。

`runner.local.yaml`、`runner.local.yml`、`runner.local.json` 不参加自动发现。它们适合私人凭证或测试覆盖；Agent 如果发现已有此类文件，应明确用 `--config PATH` 或 `COMPARE_FIGURE_CONFIG=PATH` 选择，不能默默混入默认配置。`examples/runner.yaml.example` 只是脱敏示例，也不会自动加载。`run.json`、`result.json`、`agent-state`、`session.jsonl` 等是运行状态，不是模型配置。

需要明确关闭文件配置时使用保留词：

```bash
build/compare-figure run --config none --original reference.png --out runs/figure
```

`--config none` 只关闭文件发现；CLI 参数、环境变量默认值和内置 runner 默认值仍然生效。JSON 请求中的 `"config": "none"` 具有相同含义。若没有显式 `--config`/JSON `config`，才执行上述默认发现；环境变量只负责配置文件选择，不表示所有环境变量都覆盖文件中的 API 字段。

## 推荐启动流程

1. 从 Skill 根阅读 `SKILL.md`，确认当前任务是自动 runner 还是当前 Agent 半自动绘图。
2. 在当前工作目录检查 `runner.yaml`、`runner.yml`、`runner.json`；再确认 resource root 是否已有配置。不要读取或打印私人 local 配置中的密钥。
3. 若已有可用配置，直接使用它，不重复询问 model、URL 或 key。只确认配置路径和所需键是否存在，让程序自行读取凭证。
4. 若没有配置，复制 [脱敏示例](../examples/runner.yaml.example) 为当前工作目录的 `runner.yaml`，填写模型接入信息；真实 key 优先放在环境变量中。
5. 首次启动先做不调用模型的检查：

```bash
build/compare-figure run --config PATH --original reference.png --out work/figure --check
```

`--check` 校验输入、配置解析和实际编译路线，JSON 的 `config_path` 会报告选中的配置绝对路径（没有配置时为空），不输出配置内容、不发模型请求，也不验证模型名或 API 密钥是否可用。检查通过后再去掉 `--check` 正式运行。任务输入仍由 `--original`，或 `--source-md` + `--image-ref`，以及 `--out` 指定；不要声称原图或任务输出目录能从 runner YAML 配置取得。

跨 cwd 使用时，配置文件可显式给绝对路径：

```bash
build/compare-figure run --config /path/to/project/runner.yaml \
  --original /path/to/reference.png --out /path/to/runs/figure --check
```

也可设置 `COMPARE_FIGURE_CONFIG=/path/to/project/runner.yaml`。这两种方式都按进程 cwd 解释配置内的相对路径；需要稳定跨目录运行时，配置内路径应写成适合该 cwd 的绝对路径，或从预定项目目录启动。

## 配置字段

以下是当前 runner 实际读取的配置位置。CLI 显式选项覆盖同一字段；未覆盖的字段才来自配置。模型名的优先级是 **CLI `--model` > 配置 > `OPENAI_MODEL` 默认环境变量**；`base_url`、API 协议和 key 环境变量同理按实际字段读取，不要把“配置文件选择的环境变量优先”误写成“所有 env 压过文件”。

### 模型接入

基础配置可以直接写在顶层：

```yaml
base_url: https://api.openai.com/v1
model: your-vision-tool-model
api: openai
api_key_env: FIGURE_MODEL_KEY
api_timeout: 180
stream: true
```

可选的高级形式使用 `models` registry 和 `extends`，通过 `--model NAME` 或配置的 `latex.drawing_model` 选择条目：

```yaml
models:
  base:
    base_url: https://api.openai.com/v1
    api: openai
    api_key_env: FIGURE_MODEL_KEY
  drawing:
    extends: base
    model: your-vision-tool-model
```

顶层直接配置和 registry 是两种可选组织方式，不要把它们拼成第三种字段格式。配置允许 `api_key`，但它会留在用户自行保存的配置文件内；优先用 `api_key_env`。CLI `--api-key` 和 `--api-key-file` 是本次进程的临时覆盖，不回写配置；密钥不要进入提交或运行记录。

### 绘图与会话

```yaml
drawing:
  mode: luadraw              # luadraw / mixed / latex / asymptote
session:
  max_tool_rounds: 128
  max_tokens: 131072
  context_limit: 256000
  compaction_at: 0.85
  tool_rounds_warn_ratio: 0.7
  tool_rounds_grace: 20
  prune_tool_chars: 4096
  keep_images: 3
checker_session:              # 可选，视觉复核会话预算
  max_tool_rounds: 50
```

兼容旧结构时，会话设置也可从 `latex.sessions.drawing`、`latex.sessions.checker` 读取。绘图策略也可以用 CLI `--drawing-mode` 覆盖；CLI 仍优先于配置。

### 看图、编译和复核

```yaml
view:
  image_max: 30
  pdf_max: 25
  warn_ratio: 0.7
compile:
  engine: auto                # auto / lualatex / xelatex / pdflatex
  raster_command: pdftoppm
  raster_dpi: 300
  timeout: 120
figure_check:
  enabled: false
  max_rounds: 2
  min_overlap: 0.17
  max_scale_dev: 0.1
  accept_min_submits: 8
  model: checker-model        # 可选复核模型别名
```

`figure_check` 也可放在 `latex.figure_check`；`compile` 也可放在 `latex.compile`；显式顶层位置优先。`view` 也兼容 `tools.view`。`min_overlap` 是比例值（`0.17` 即 17%），CLI `--pass-threshold` 则使用百分比数值（`17`）。

### 文档频次提示

```yaml
docs:
  cache_dir: work/doc-cache
  frequent:
    prompt_enabled: true
    prompt_max_chars: 2048
    prompt_lines: 8
    line_chars: 256
```

`docs.frequent` 只控制新 runner 会话开头的 `document_search` 高频摘要；主动调用工具仍使用 `document_search`（旧 `luadraw_doc` 兼容）。缓存可由 CLI/环境单独指定。

## 不要混淆的边界

- `--model`、`--base-url`、`--api`、`--api-key-env`、`--api-key` 等 CLI 只覆盖本次 run；配置文件不会替代任务输入参数。
- `--original`、`--source-md`、`--image-ref` 和 `--out` 是本次任务的输入/输出选项，不是 runner YAML 字段来源。
- `compile`、`docs`、`compare`、`environment` 独立命令不读取 runner 配置；需要它们的选项请直接传 CLI/JSON 参数。
- 自动发现不扫描父目录或用户 HOME，也不读取 `runner.local.*`，避免不同项目或私人测试配置串用。
