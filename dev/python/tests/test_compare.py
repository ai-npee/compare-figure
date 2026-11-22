import ast
import importlib.util
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
ALGORITHM_SOURCE = ROOT / "compare.py"


def load_threshold_function():
    tree = ast.parse(ALGORITHM_SOURCE.read_text(encoding="utf-8"))
    node = next(n for n in tree.body if isinstance(n, ast.FunctionDef) and n.name == "threshold_for")
    module = ast.Module(body=[node], type_ignores=[])
    namespace = {}
    exec(compile(module, str(ALGORITHM_SOURCE), "exec"), namespace)
    return namespace["threshold_for"]


def load_algorithm():
    spec = importlib.util.spec_from_file_location("figure_compare_algorithm_test", ALGORITHM_SOURCE)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class CompareFigureTests(unittest.TestCase):
    def test_threshold_policy(self):
        threshold_for = load_threshold_function()
        self.assertEqual(threshold_for(), 50.0)
        self.assertGreater(threshold_for(0, 1), 50.0)
        self.assertGreater(threshold_for(0.5, 0), threshold_for(0.5, 1))
        self.assertLess(threshold_for(1, 0.5), 50.0)
        self.assertEqual(threshold_for(1, 0, minimum=50, maximum=62), 50.0)
        self.assertEqual(threshold_for(0, 0, minimum=47, maximum=60), 60.0)
        self.assertEqual(threshold_for(-4, 9), threshold_for(0, 1))
        with self.assertRaises(ValueError):
            threshold_for(minimum=70, maximum=40)

    @unittest.skipUnless(importlib.util.find_spec("numpy") and importlib.util.find_spec("PIL"), "requires NumPy and Pillow")
    def test_small_grid_levels_have_positive_cell_areas(self):
        import numpy as np

        module = load_algorithm()
        for canvas in (24, 28, 31, 32, 33, 64, 96):
            with self.subTest(canvas=canvas):
                finest = module.parse_finest("0.95S", canvas)
                levels = module.levels_for(canvas, finest)
                self.assertEqual(levels[-1], min(canvas, finest))
                for grid in levels:
                    self.assertTrue(np.all(np.diff(np.arange(grid + 1) * canvas // grid) > 0))
                ink = np.ones((canvas, canvas, 3), dtype=np.float32)
                with np.errstate(divide="raise", invalid="raise"):
                    result = module.progressive_overlap(ink, ink)
                self.assertAlmostEqual(result["score"], 1.0)
                self.assertEqual(result["shift_px"], {"x": 0, "y": 0})
        self.assertEqual(module.levels_for(96, 91), [32, 64, 91])

    @unittest.skipUnless(importlib.util.find_spec("numpy") and importlib.util.find_spec("PIL"), "requires NumPy and Pillow")
    def test_distinct_alignment_starts_are_preserved(self):
        import numpy as np

        module = load_algorithm()
        ink = np.ones((24, 28, 3), dtype=np.float32)
        with np.errstate(divide="raise", invalid="raise"):
            result = module.progressive_overlap(ink, ink, starts=[(0, 0), (1, 0), (1, 0)])
        self.assertEqual(result["alignment_starts"], [{"y": 0, "x": 0}, {"y": 1, "x": 0}])
        self.assertEqual(len(result["alignment_runs"]), 2)
        self.assertEqual(result["shift_px"], {"x": 0, "y": 0})
        self.assertAlmostEqual(result["score"], 1.0)

    @unittest.skipUnless(importlib.util.find_spec("numpy") and importlib.util.find_spec("PIL"), "requires NumPy and Pillow")
    def test_asymmetric_content_origin_compensation(self):
        import numpy as np
        from PIL import Image

        module = load_algorithm()
        reference = np.full((96, 96, 3), 255, dtype=np.uint8)
        candidate = np.full_like(reference, 255)
        reference[40:64, 30:58] = 0
        candidate[32:56, 29:57] = 0
        with tempfile.TemporaryDirectory() as td:
            td = Path(td)
            ref_path, cand_path = td / "reference.png", td / "candidate.png"
            Image.fromarray(reference).save(ref_path)
            Image.fromarray(candidate).save(cand_path)
            with np.errstate(divide="raise", invalid="raise"):
                metrics, _ = module.save_comparisons(ref_path, cand_path, td / "out")
            aligned = module.translated(candidate, metrics["translation_px"]["x"], metrics["translation_px"]["y"])
            np.testing.assert_array_equal(aligned, reference)
        self.assertEqual(metrics["translation_px"], {"x": 1, "y": 8})
        self.assertAlmostEqual(metrics["similarity_percent"], 100.0)
        # Both cropped rectangles have the same centroid: the centroid and
        # origin starts coincide and must be deduplicated, not counted twice.
        self.assertEqual([run["start"] for run in metrics["alignment_runs"]], [{"y": 0, "x": 0}])
        self.assertEqual(len(metrics["alignment_runs"]), 1)


if __name__ == "__main__":
    unittest.main()
