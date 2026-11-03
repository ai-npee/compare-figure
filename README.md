# compare-figure

图形比较算法：比较参考图片与候选 PDF，输出相似度、多通道叠图和局部差异。当前为从 DocVision 提取的初始化版本。

## 文件

| 文件 | 用途 |
| --- | --- |
| [compare.py](compare.py) | 唯一 Python 主实现：PDF 栅格化、对齐、评分与图像输出 |
| [Go 实现](go/compare.go) | 对应的图像对齐、评分、叠图算法，保留原算法函数 |
| [viewer.html](viewer.html) | 离线查看指标与比较图片 |
| [容错评分](algorithms/robust_metrics.py) | 对微小位移、抗锯齿和线宽变化更宽容的附加指标 |
| [校准工具](algorithms/calibrate_metrics.py) | 合成扰动样例与阈值扫描，研发用途 |
| [开发记录](docs/notes.md) | 来源、算法边界与已知问题 |

主算法和附加算法分开维护。仓库不包含模型调用、会话维护或 DocVision 编译流程。

## 使用

Python 3.10+，依赖 NumPy、Pillow；PDF 栅格化需要 Poppler 的 `pdfinfo`、`pdftocairo`。本工作区使用 conda base：

```bash
conda run -n base python -m pip install -r requirements.txt
conda run -n base python compare.py --original /path/to/reference.png --pdf /path/to/candidate.pdf --out runs/example
conda run -n base python compare.py --help
```

浏览器打开 [viewer.html](viewer.html)，选择输出目录中的 `metrics.json` 及 PNG 文件。网页离线读取，不上传。主要输出为候选栅格图、灰度／RGB 叠图、四联图、局部图及指标 JSON。

## 算法

候选按参考图宽度等比缩放，以墨迹内容裁切原点和质心辅助多尺度平移搜索。主分是 RGB 通道 soft-ink Jaccard 的均值，灰度分单独报告；同时生成最差区域和叠图供复查。不进行旋转、透视或语义识别。

默认通过门槛 50%。可选 `adaptive-v1` 根据显式提供的难度与置信度调整门槛，只影响 pass，不改变分数。视觉分数不代表标签或数学语义正确。

附加容错评分提供局部墨迹覆盖率与粗结构重叠，未接入主算法。校准工具的入口为 `python -m algorithms.calibrate_metrics --out runs/calibration`，目前有已知接口问题，见开发记录。

## 开发

```bash
conda run -n base env PYTHONPATH=.:algorithms python -m unittest discover -s tests -v
```

Go 只保留算法与原有算法测试，移除了会话工具适配层，仍使用 `golang.org/x/image/draw` 做图像缩放。本轮不安装 Go 依赖、不新增模块或 CLI、不运行 Go 验证。

Python 核心现有小尺寸网格回归失败尚未修复；本轮仅整理提取范围。来源与问题集中于 [开发记录](docs/notes.md)，版权沿用 [MIT LICENSE](LICENSE)。
