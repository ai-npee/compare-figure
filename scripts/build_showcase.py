#!/usr/bin/env python3
"""Build a PDF/SVG comparison showcase with XeLaTeX and Poppler."""

from __future__ import annotations

import argparse
import json
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

import fitz

PAGE_W = 1200
PAGE_H = 580


def visible_bbox(pdf_path: Path) -> tuple[float, float, float, float, float, float]:
    doc = fitz.open(pdf_path)
    if len(doc) != 1:
        raise ValueError(f"Expected a single-page vector PDF, found {len(doc)} pages")
    page = doc[0]
    if page.get_images():
        raise ValueError("The supplied redrawn PDF contains raster images")
    pix = page.get_pixmap(matrix=fitz.Matrix(2, 2), alpha=False)
    channels = pix.n
    samples = pix.samples
    min_x, min_y, max_x, max_y = pix.width, pix.height, -1, -1
    for y in range(pix.height):
        row = y * pix.width * channels
        for x in range(pix.width):
            i = row + x * channels
            if min(samples[i], samples[i + 1], samples[i + 2]) < 245:
                min_x, min_y = min(min_x, x), min(min_y, y)
                max_x, max_y = max(max_x, x), max(max_y, y)
    if max_x < 0:
        raise ValueError("The supplied PDF page appears blank")
    sx, sy = pix.width / page.rect.width, pix.height / page.rect.height
    pad = 2
    x0, y0 = max(0, min_x - pad) / sx, max(0, min_y - pad) / sy
    x1 = min(pix.width, max_x + 1 + pad) / sx
    y1 = min(pix.height, max_y + 1 + pad) / sy
    width, height = page.rect.width, page.rect.height
    doc.close()
    return x0, height - y1, width - x1, y0


def fontspec_setup(font_path: Path | None) -> str:
    if font_path is None:
        return r"\setmainfont{Noto Sans CJK SC}\setsansfont{Noto Sans CJK SC}"
    font_path = font_path.resolve(strict=True)
    suffix = font_path.suffix
    if suffix.lower() not in {".ttf", ".otf", ".ttc"}:
        raise ValueError("--font must point to a TTF, OTF, or TTC font file")
    directory = font_path.parent.as_posix().replace("\\", "/") + "/"
    stem = font_path.stem
    index = ",FontIndex=2" if suffix.lower() == ".ttc" else ""
    return (
        "\\setmainfont{" + stem + "}[Path={" + directory + "},Extension=" + suffix
        + ",UprightFont=*" + index + "]"
        "\\setsansfont{" + stem + "}[Path={" + directory + "},Extension=" + suffix
        + ",UprightFont=*" + index + "]"
    )


def tex_source(original_name: str, redrawn_name: str,
               trim: tuple[float, float, float, float], font_setup: str) -> str:
    left, bottom, right, top = (f"{v:.4f}bp" for v in trim)
    return rf"""\documentclass[tikz,border=0pt]{{standalone}}
\usepackage[paperwidth=1200bp,paperheight=580bp,margin=0bp]{{geometry}}
\usepackage{{fontspec,graphicx,tikz,xcolor}}
\usetikzlibrary{{arrows.meta}}
{font_setup}
\definecolor{{ink}}{{HTML}}{{1F2E3B}}
\definecolor{{muted}}{{HTML}}{{616F7A}}
\definecolor{{accent}}{{HTML}}{{0A6E66}}
\definecolor{{arrow}}{{HTML}}{{E85C2E}}
\begin{{document}}
\begin{{tikzpicture}}[x=1bp,y=1bp]
  \path[use as bounding box] (0,0) rectangle (1200,580);
  \begin{{scope}}[shift={{(72bp,-70bp)}}]
  \node[inner sep=0pt] at (239,265) {{\includegraphics[width=382bp]{{{original_name}}}}};
  \node[inner sep=0pt] at (961,265) {{\includegraphics[width=382bp,trim={left} {bottom} {right} {top},clip]{{{redrawn_name}}}}};
  \node[anchor=base west,text=ink,font=\sffamily\fontsize{{21}}{{25}}\selectfont] at (68,487) {{原始位图}};
  \node[anchor=base west,text=muted,font=\sffamily\fontsize{{10}}{{12}}\selectfont] at (68,465) {{REFERENCE IMAGE}};
  \node[anchor=base west,text=ink,font=\sffamily\fontsize{{21}}{{25}}\selectfont] at (789,487) {{矢量重绘}};
  \node[anchor=base west,text=muted,font=\sffamily\fontsize{{10}}{{12}}\selectfont] at (789,465) {{VECTOR PDF}};
  \node[anchor=base west,text=ink,font=\sffamily\bfseries\fontsize{{22}}{{26}}\selectfont] at (473,310) {{compare-figure}};
  \draw[arrow,line width=2bp,-{{Stealth[length=9bp,width=7bp]}}] (490,282) -- (710,282);
  \node[anchor=base west,text=accent,font=\sffamily\fontsize{{15}}{{18}}\selectfont] at (501,243) {{Agent 调用工具}};
  \node[anchor=base west,text=muted,font=\sffamily\fontsize{{13}}{{16}}\selectfont] at (504,216) {{查文档 · 绘制}};
  \node[anchor=base west,text=muted,font=\sffamily\fontsize{{13}}{{16}}\selectfont] at (504,193) {{编译 · 检查}};
  \draw[muted!22,line width=0.7bp] (68,63) -- (1132,63);
  \node[anchor=base east,text=muted,font=\sffamily\fontsize{{12}}{{14}}\selectfont] at (1132,34) {{LuaDraw · PDF 矢量输出}};
  \end{{scope}}
\end{{tikzpicture}}
\end{{document}}
"""


def build(original: Path, redrawn: Path, out_dir: Path, font_path: Path | None,
          build_dir: Path) -> dict[str, object]:
    original = original.resolve(strict=True)
    redrawn = redrawn.resolve(strict=True)
    out_dir.mkdir(parents=True, exist_ok=True)
    out_dir = out_dir.resolve(strict=True)
    build_dir.mkdir(parents=True, exist_ok=True)
    build_dir = build_dir.resolve(strict=True)
    pdf_out, svg_out = out_dir / "two-tori.pdf", out_dir / "two-tori.svg"
    if pdf_out.resolve() in {original, redrawn} or svg_out.resolve() in {original, redrawn}:
        raise ValueError("Output files must not overwrite either input")

    trim = visible_bbox(redrawn)[:4]
    setup = fontspec_setup(font_path)
    with tempfile.TemporaryDirectory(prefix="compare-figure-", dir=build_dir) as temp_name:
        temp = Path(temp_name)
        original_copy = temp / ("reference" + original.suffix.lower())
        redrawn_copy = temp / "redrawn.pdf"
        shutil.copyfile(original, original_copy)
        shutil.copyfile(redrawn, redrawn_copy)
        tex_file = temp / "two-tori.tex"
        tex_file.write_text(tex_source(original_copy.name, redrawn_copy.name, trim, setup), encoding="utf-8")
        cmd = ["xelatex", "-no-shell-escape", "-halt-on-error", "-interaction=nonstopmode",
               "-output-directory", str(temp), str(tex_file)]
        result = subprocess.run(cmd, cwd=temp, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        compiled_pdf = temp / "two-tori.pdf"
        if result.returncode or not compiled_pdf.is_file():
            raise RuntimeError("XeLaTeX failed:\n" + "\n".join(result.stdout.splitlines()[-40:]))
        converted_svg = temp / "two-tori.svg"
        svg_cmd = ["pdftocairo", "-svg", "-f", "1", "-l", "1", str(compiled_pdf), str(converted_svg)]
        conversion = subprocess.run(svg_cmd, cwd=temp, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        if conversion.returncode or not converted_svg.is_file():
            raise RuntimeError("pdftocairo SVG conversion failed:\n" + conversion.stdout)
        shutil.copyfile(compiled_pdf, pdf_out)
        shutil.copyfile(converted_svg, svg_out)

    return {
        "pdf": str(pdf_out),
        "svg": str(svg_out),
        "pdf_bytes": pdf_out.stat().st_size,
        "svg_bytes": svg_out.stat().st_size,
        "source_image": str(original),
        "source_pdf": str(redrawn),
        "pdf_engine": "XeLaTeX",
        "svg_engine": "pdftocairo",
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--original", type=Path, required=True)
    parser.add_argument("--redrawn", type=Path, required=True)
    parser.add_argument("--font", type=Path, help="Optional TTF/OTF/TTC font file for fontspec")
    parser.add_argument("--out-dir", type=Path, default=Path("assets/showcase"))
    parser.add_argument("--build-dir", type=Path, default=Path("build/showcase-validation"))
    args = parser.parse_args()
    try:
        result = build(args.original, args.redrawn, args.out_dir, args.font, args.build_dir)
        print(json.dumps(result, ensure_ascii=False, indent=2))
    except Exception as exc:
        print(f"build_showcase: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
