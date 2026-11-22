package drawing

import (
	"compare-figure/measurement"
	"encoding/json"
	"os"
)

// Measurement is shared with the public comparison API, so runner, CLI and
// model tools use identical sizes.csv discovery and physical-size precedence.
type Measurement = measurement.Measurement

func MeasureImage(path, parseRoot string, referenceMM []float64) (Measurement, error) {
	return measurement.MeasureImage(path, parseRoot, referenceMM)
}
func readJSON(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}
