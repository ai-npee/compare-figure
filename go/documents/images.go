package documents

import (
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"

	"doc-library/pkg/doclibrary"
	"golang.org/x/image/draw"
)

// The legacy zoom is the final crop width. Ask the library to render enough
// pixels before cropping, then normalize only the returned PNG's display size.
// PDF extraction and region selection remain entirely in doc-library.
func cropResolution(request Request) (Request, int) {
	if request.DPI > 0 || request.Region == nil || !(request.Mode == "view" || request.IncludeImage) {
		return request, 0
	}
	region := request.Region
	fraction := (region.Right - region.Left) / 100
	if math.IsNaN(fraction) || math.IsInf(fraction, 0) || fraction <= 0 || fraction >= 1 {
		return request, 0
	}
	width := request.Zoom
	if width == 0 {
		width = 1200
	}
	if width < 1 || width > 5000 {
		return request, 0
	}
	request.Zoom = int(math.Min(5000, math.Ceil(float64(width)/fraction)))
	return request, width
}

func resizeImages(images []doclibrary.Image, width int) error {
	for i := range images {
		artifact := &images[i]
		if artifact.Width == width {
			continue
		}
		f, err := os.Open(artifact.Path)
		if err != nil {
			return err
		}
		source, err := png.Decode(f)
		f.Close()
		if err != nil {
			return err
		}
		bounds := source.Bounds()
		height := max(1, int(math.Round(float64(bounds.Dy())*float64(width)/float64(bounds.Dx()))))
		target := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.CatmullRom.Scale(target, target.Bounds(), source, bounds, draw.Src, nil)
		// This path is a newly created private preview returned by the library.
		f, err = os.OpenFile(artifact.Path, os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		err = png.Encode(f, target)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		artifact.Width = width
		artifact.Height = height
		if artifact.Base64 != "" {
			data, err := os.ReadFile(artifact.Path)
			if err != nil {
				return fmt.Errorf("inline resized document: %w", err)
			}
			artifact.Base64 = base64.StdEncoding.EncodeToString(data)
		}
	}
	return nil
}
