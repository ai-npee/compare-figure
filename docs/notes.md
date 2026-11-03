# 开发记录

## 来源

来源为 [DocVision](https://github.com/LingNc/DocVision)，提取版本 `3761927db079db7e10aa9fbf560fe9ddc8960bb4`，保留 MIT 版权。

- [compare.py](../compare.py)：来自 `figure_compare/algorithms/compare.py`，原样保留；删除了与它内容一致的 Skill 副本。
- [viewer.html](../viewer.html)：来自 `figure_compare/compare_figure_skill/viewer.html`，原样保留。它是比较结果查看器。
- [Go 算法](../go/compare.go)：来自 `go/internal/latex/figure_compare.go`，提取从 flattenOnWhite 起的算法函数，去除 formatAlignmentRuns 展示辅助函数，调整包名与 imports；算法函数体保持原样。未新增公共 API 或命令行入口。
- [Go 算法测试](../go/compare_test.go)：仅保留平移、叠图与裁切测试，去除会话工具测试。
- [附加算法](../algorithms/robust_metrics.py)与[校准工具](../algorithms/calibrate_metrics.py)：来自 `figure_compare/algorithms/`；后者只调整了附加指标的 import 路径。
- Python 测试保留核心阈值／平移回归及容错指标回归，删除针对重复 Skill 布局的断言。

会话 runner、会话网页、停止策略、外部 judge 合约和重复的 vnext 评估入口不在当前范围内。未改动源仓库或工作区旧 Skill 导出。

## 指标边界

传统主分是有限画布上的 RGB 墨迹重叠均值。附加指标包括严格灰度 Jaccard、默认 2 像素半径的容错覆盖率，以及默认 64 格的粗结构重叠。综合视觉分默认为 0.65×容错覆盖率 + 0.35×粗结构重叠，不等于主分。

动态门槛为 `clamp(base + 10*(0.5-confidence) - 12*(difficulty-0.5), min, max)`，默认 base=50、min=40、max=75，仍属待校准启发式。Go 为对应传统算法的来源实现，未实现全部 Python CLI 功能，也未验证跨语言数值一致性。

## 已知问题

首次提取时在本仓库和源仓库的 conda base 环境中均复现：

- 核心小图回归 test_asymmetric_content_origin_compensation 期望平移 (1,8)，实际 (-2,5)，伴随网格除零／无效值警告；保持原实现，待后续修复。
- 校准工具仍把 content_ink 返回的 tuple 传给 progressive_overlap，接口尚未更新；只保留为后续研发工具，未修复或宣称可用。

本轮不验证已排除的会话代码，也不进行 Go 构建／运行测试。文件整理只核对来源、路径、导入和文档链接。

## 维护

保持一个 Python 主实现。附加算法独立于主实现，功能合并前明确指标兼容性。使用 conda base；真实素材与运行结果放忽略的 runs 目录，预览放预览目录。重要变更同步 README 和 AGENTS，后续开发方案按用户要求再定。
