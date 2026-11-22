import sys
import unittest
from pathlib import Path

import numpy as np
from PIL import Image, ImageDraw

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from vector_metrics import _catmull_rom_resize, _grid, compare_images, make_overlay, worst_region


def ink_on(w, h, x0, y0, x1, y1):
    image = Image.new("RGB", (w, h), "white")
    ImageDraw.Draw(image).rectangle((x0, y0, x1 - 1, y1 - 1), fill="black")
    return image


class VectorMetricsTests(unittest.TestCase):
    def test_catmull_rom_fixed_rgb_fixture(self):
        src = np.array([[[0, 20, 240], [100, 80, 40], [255, 128, 0]],
                        [[30, 200, 100], [220, 10, 50], [70, 180, 230]]], dtype=np.uint8)
        expected = np.array([
            [[0, 2, 255], [23, 33, 178], [92, 84, 39], [210, 114, 0], [255, 128, 0]],
            [[4, 115, 180], [66, 82, 123], [160, 45, 45], [168, 113, 85], [163, 162, 120]],
            [[17, 228, 94], [108, 131, 68], [228, 5, 50], [126, 112, 175], [45, 197, 255]],
        ], dtype=np.uint8)
        np.testing.assert_array_equal(_catmull_rom_resize(src, 5, 3), expected)

    def test_catmull_rom_same_size_is_exact_copy_path(self):
        src = np.arange(4*3*3, dtype=np.uint8).reshape((3, 4, 3))
        self.assertIs(_catmull_rom_resize(src, 4, 3), src)

    def test_grid_large_odd_dimensions_use_truncating_offsets(self):
        ink = np.zeros((515, 517, 3), dtype=np.float64)
        ink[1, 1] = 1.0
        ink[1, 2] = 1.0
        grid = _grid(ink, 512, 32)
        # Go int division gives offX=-2, offY=-1; x=1 is clipped, x=2 maps in.
        self.assertEqual(grid[0, 0, 0], 1 / 256)
        self.assertEqual(grid[0, 0, 1], 1 / 256)
        self.assertEqual(grid[0, 0, 2], 1 / 256)

    def test_missing_ink_zeros_aspect_metrics(self):
        blank = Image.new("RGB", (20, 20), "white")
        marked = ink_on(20, 20, 5, 5, 15, 15)
        result = compare_images(blank, marked)
        self.assertEqual((result["aspect_ref"], result["aspect_cand"], result["aspect_dev"]), (0, 0, 0))

    def test_physical_measurement_is_resolution_independent(self):
        ref = ink_on(200, 100, 50, 25, 150, 75)
        cand = ink_on(1000, 500, 250, 125, 750, 375)
        result = compare_images(ref, cand, (20, 10, 20, 10))
        self.assertEqual(result["size_unit"], "mm")
        self.assertEqual((result["scale_x"], result["scale_y"], result["scale_dev"]), (1, 1, 0))
        self.assertEqual((result["ink_ref_w"], result["ink_cand_w"]), (10, 10))
        self.assertEqual((result["aspect_ref"], result["aspect_cand"]), (2, 2))

    def test_physical_size_detects_scale_and_aspect_fallback(self):
        ref = ink_on(200, 100, 50, 25, 150, 75)
        larger = ink_on(400, 200, 100, 50, 300, 150)
        result = compare_images(ref, larger, (20, 10, 40, 20))
        self.assertEqual(result["scale_dev"], 1)
        squash = ink_on(400, 200, 0, 0, 400, 100)
        fallback = compare_images(ref, squash)
        self.assertEqual(fallback["size_unit"], "aspect")
        self.assertAlmostEqual(fallback["scale_dev"], 1)

    def test_bbox_and_size_are_measured_before_fit(self):
        ref, cand = ink_on(100, 100, 25, 25, 75, 75), ink_on(200, 100, 50, 25, 150, 75)
        out = compare_images(ref, cand, (20, 20, 20, 20))
        self.assertEqual((out["ink_ref_w"], out["ink_cand_w"]), (10, 10))
        self.assertEqual((out["scale_x"], out["scale_y"]), (1, 1))
        self.assertGreaterEqual(out["canvas"], 24)

    def test_translation_runs_scores_and_overlay_dimensions(self):
        ref, cand = Image.new("RGB", (96, 80), "white"), Image.new("RGB", (96, 80), "white")
        ImageDraw.Draw(ref).rectangle((18, 12, 70, 60), fill=(30, 30, 30))
        ImageDraw.Draw(cand).rectangle((22, 9, 74, 57), fill=(30, 30, 30))
        result = compare_images(ref, cand)
        self.assertGreater(result["similarity"], .75)
        for value in result["channels"]:
            self.assertAlmostEqual(value, result["similarity"])
        self.assertEqual(result["runs"][0]["name"], "origin")
        self.assertIn(len(result["runs"]), (1, 2))
        self.assertTrue(all("score" in r and "dx" in r and "dy" in r for r in result["runs"]))
        self.assertEqual(make_overlay(ref, cand, result["dx"], result["dy"], "all").size, (192, 160))
        self.assertEqual(make_overlay(ref, cand, 0, 0, "rg").size, ref.size)

    def test_overlay_colors_and_worst_region(self):
        ref, cand = Image.new("RGB", (20, 20), "white"), Image.new("RGB", (20, 20), "white")
        ImageDraw.Draw(ref).rectangle((1, 1, 4, 4), fill="black")
        ImageDraw.Draw(cand).rectangle((10, 10, 13, 13), fill="black")
        overlay = np.asarray(make_overlay(ref, cand, 0, 0, "r"))
        self.assertEqual(tuple(overlay[2, 2]), (255, 0, 255))
        self.assertEqual(tuple(overlay[11, 11]), (0, 255, 0))
        self.assertIn(worst_region(ref, cand), ((0., 0., 50., 50.), (50., 0., 100., 50.),
                                               (0., 50., 50., 100.), (50., 50., 100., 100.)))


if __name__ == "__main__":
    unittest.main()
