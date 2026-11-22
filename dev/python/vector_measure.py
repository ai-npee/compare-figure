"""Printed figure measurements ported from DocVision imagescale.go (20faca6)."""
from dataclasses import dataclass, asdict, replace
from functools import lru_cache
import json
from pathlib import Path

from PIL import Image

LAYOUT_SCALE = 2.0
PT_PER_MM = 72 / 25.4


@dataclass
class ImageMeasure:
    width_mm: float = 0
    height_mm: float = 0
    page_w_mm: float = 0
    pixels_w: int = 0
    pixels_h: int = 0
    dpi: int = 0
    found: bool = False
    aspect_str: str = ""

    def hint(self):
        if self.found:
            part = f", about {self.width_mm / self.page_w_mm * 100:.0f}% of the page width" if self.page_w_mm > 0 else ""
            return (f"ORIGINAL FIGURE SIZE: {self.width_mm:.1f}mm x {self.height_mm:.1f}mm on the page{part}; "
                    f"bitmap {self.pixels_w}x{self.pixels_h}px, effective resolution {self.dpi} dpi, aspect {self.aspect_str}.")
        return f"ORIGINAL BITMAP: {self.pixels_w}x{self.pixels_h}px, aspect {self.aspect_str}." if self.aspect_str else ""

    def crop(self, left, top, right, bottom):
        if not self.found or self.pixels_w <= 0 or self.pixels_h <= 0:
            return ImageMeasure()
        fw, fh = (right - left) / 100, (bottom - top) / 100
        if fw <= 0 or fh <= 0:
            return ImageMeasure()
        out = replace(self, pixels_w=int(self.pixels_w * fw), pixels_h=int(self.pixels_h * fh), width_mm=self.width_mm * fw)
        out.height_mm = out.width_mm * out.pixels_h / out.pixels_w if self.width_mm > 0 and out.pixels_w > 0 else self.height_mm * fh
        return out

    def crop_hint(self):
        return f"this crop is {self.width_mm:.1f}mm x {self.height_mm:.1f}mm on the page" if self.found else ""

    def to_dict(self):
        return asdict(self)


def _json(path, default):
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return default


def _entries(part):
    for file in sorted(part.glob("*content_list.json")):
        values = _json(file, [])
        if isinstance(values, list):
            yield from (v for v in values if isinstance(v, dict))


def _entry_measure(entry, part):
    bbox = entry.get("bbox", [])
    if len(bbox) != 4 or not entry.get("img_path"):
        return None
    width = (bbox[2] - bbox[0]) / LAYOUT_SCALE / PT_PER_MM
    height = (bbox[3] - bbox[1]) / LAYOUT_SCALE / PT_PER_MM
    if width <= 0 or height <= 0:
        return None
    pages = _json(part / "layout.json", {}).get("pdf_info", [])
    idx = entry.get("page_idx", 0)
    page = pages[idx].get("page_size", [0, 0]) if 0 <= idx < len(pages) else [0, 0]
    return {"width_mm": width, "height_mm": height, "page_w_mm": page[0] / PT_PER_MM if page[0] > 0 else 0,
            "aspect_str": f"{width / height:.2f}:1"}


@lru_cache(maxsize=8)
def _parse_index(root):
    index = {}
    for part in sorted(Path(root).glob("*")):
        if not part.is_dir():
            continue
        for entry in _entries(part):
            result = _entry_measure(entry, part)
            if result:
                index.setdefault(Path(entry["img_path"]).name.lower(), result)
    return index


def measure_image(path, parse_root="", reference_mm=None):
    path = Path(path)
    result = ImageMeasure()
    try:
        with Image.open(path) as image:
            result.pixels_w, result.pixels_h = image.size
        if result.pixels_w > 0 and result.pixels_h > 0:
            result.aspect_str = f"{result.pixels_w / result.pixels_h:.2f}:1"
    except OSError:
        pass
    found = None
    parent = path.resolve().parent
    part = None
    for _ in range(5):
        if parent == parent.parent:
            break
        if next(parent.glob("*content_list.json"), None):
            part = parent
            break
        parent = parent.parent
    if part:
        for entry in _entries(part):
            if Path(entry.get("img_path", "")).name.lower() == path.name.lower():
                found = _entry_measure(entry, part)
                break
    elif parse_root:
        found = _parse_index(str(Path(parse_root).resolve())).get(path.name.lower())
    if reference_mm and len(reference_mm) >= 2 and reference_mm[0] > 0 and reference_mm[1] > 0:
        # Independent-input adapter: explicit original canvas size has precedence.
        found = {"width_mm": reference_mm[0], "height_mm": reference_mm[1], "page_w_mm": 0}
        result.width_mm, result.height_mm = found["width_mm"], found["height_mm"]
    elif found:
        result.width_mm = found["width_mm"]
        result.height_mm = result.width_mm * result.pixels_h / result.pixels_w if result.pixels_w > 0 and result.pixels_h > 0 else found["height_mm"]
    if found:
        result.found = True
        result.page_w_mm = found.get("page_w_mm", 0)
        if result.pixels_w > 0 and result.width_mm > 0:
            result.dpi = int(result.pixels_w / (result.width_mm / 25.4))
        if not result.aspect_str:
            result.aspect_str = found.get("aspect_str", "")
    return result
