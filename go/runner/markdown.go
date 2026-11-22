package runner

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type markdownImageRef struct {
	raw      string
	decoded  string
	line     int
	resolved string
}

// ResolveMarkdown resolves an image referenced by a Markdown source without
// changing the source document. It returns the selected absolute image path,
// the original Markdown reference, the image root used by the harness, and a
// small context window around the selected image.
func ResolveMarkdown(sourceMD, imageRef, original, imagesDir, contextText string) (resolvedOriginal, resolvedRef, resolvedImages, resolvedContext string, err error) {
	if sourceMD == "" {
		return original, imageRef, imagesDir, contextText, nil
	}

	mdPath, err := filepath.Abs(sourceMD)
	if err != nil {
		return "", "", "", "", fmt.Errorf("resolve source markdown: %w", err)
	}
	data, err := os.ReadFile(mdPath)
	if err != nil {
		return "", "", "", "", fmt.Errorf("read source markdown %q: %w", mdPath, err)
	}
	mdDir := filepath.Dir(mdPath)
	if imagesDir == "" {
		imagesDir = filepath.Join(mdDir, "images")
	} else if imagesDir, err = filepath.Abs(imagesDir); err != nil {
		return "", "", "", "", fmt.Errorf("resolve images directory: %w", err)
	}
	resolvedImages = imagesDir

	refs, lines := parseMarkdownImages(string(data))
	locals := make([]markdownImageRef, 0, len(refs))
	for _, ref := range refs {
		if isExternalImageRef(ref.decoded) {
			continue
		}
		candidate := resolveMarkdownImage(mdDir, imagesDir, ref.decoded)
		if candidate != "" {
			ref.resolved = candidate
			locals = append(locals, ref)
		}
	}

	requested := normalizeImageRef(imageRef)
	var selected *markdownImageRef
	if imageRef != "" {
		if isExternalImageRef(requested) {
			return "", "", resolvedImages, contextText, fmt.Errorf("image-ref %q is an external URL; only local Markdown images are supported", imageRef)
		}
		for i := range locals {
			if normalizeImageRef(locals[i].decoded) == requested {
				if selected != nil {
					return "", "", resolvedImages, contextText, fmt.Errorf("image-ref %q matches multiple local images", imageRef)
				}
				selected = &locals[i]
			}
		}
		if selected == nil {
			base := filepath.Base(filepath.FromSlash(requested))
			matches := make([]*markdownImageRef, 0, 2)
			for i := range locals {
				if filepath.Base(filepath.FromSlash(normalizeImageRef(locals[i].decoded))) == base {
					matches = append(matches, &locals[i])
				}
			}
			if len(matches) == 1 {
				selected = matches[0]
			} else if len(matches) > 1 {
				return "", "", resolvedImages, contextText, fmt.Errorf("image-ref %q matches duplicate basename %q; use the complete reference", imageRef, base)
			}
		}
		if selected == nil {
			return "", "", resolvedImages, contextText, fmt.Errorf("image-ref %q was not found as a local Markdown image", imageRef)
		}
	} else if original != "" {
		if isExternalImageRef(original) {
			return "", "", resolvedImages, contextText, fmt.Errorf("original %q is an external URL; only local Markdown images are supported", original)
		}
		wanted, e := filepath.Abs(original)
		if e != nil {
			return "", "", resolvedImages, contextText, fmt.Errorf("resolve original %q: %w", original, e)
		}
		wanted = realPath(wanted)
		for i := range locals {
			if realPath(locals[i].resolved) == wanted {
				selected = &locals[i]
				break
			}
		}
		if selected == nil {
			return "", "", resolvedImages, contextText, fmt.Errorf("original %q is not referenced by a local image in %s", original, mdPath)
		}
	} else {
		if len(locals) != 1 {
			if len(locals) == 0 {
				return "", "", resolvedImages, contextText, fmt.Errorf("Markdown contains no resolvable local images; specify image-ref")
			}
			return "", "", resolvedImages, contextText, fmt.Errorf("Markdown contains %d local images; specify image-ref", len(locals))
		}
		selected = &locals[0]
	}

	resolvedOriginal = selected.resolved
	resolvedRef = selected.raw
	start := selected.line - 20
	if start < 0 {
		start = 0
	}
	end := selected.line + 21
	if end > len(lines) {
		end = len(lines)
	}
	window := strings.Join(lines[start:end], "\n")
	resolvedContext = contextText
	if window != "" {
		if resolvedContext != "" {
			resolvedContext += "\n\n"
		}
		resolvedContext += window
	}
	return resolvedOriginal, resolvedRef, resolvedImages, resolvedContext, nil
}

func isExternalImageRef(ref string) bool {
	lower := strings.ToLower(strings.TrimSpace(ref))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "data:")
}

func normalizeImageRef(ref string) string {
	ref = strings.TrimSpace(ref)
	ref = strings.Trim(ref, "<>")
	if i := strings.IndexAny(ref, "?#"); i >= 0 {
		ref = ref[:i]
	}
	if decoded, err := url.PathUnescape(ref); err == nil {
		ref = decoded
	}
	return filepath.ToSlash(strings.ReplaceAll(ref, "\\", "/"))
}

func resolveMarkdownImage(mdDir, imagesDir, ref string) string {
	normalized := normalizeImageRef(ref)
	if normalized == "" || filepath.IsAbs(filepath.FromSlash(normalized)) {
		candidate := filepath.FromSlash(normalized)
		if fileExists(candidate) {
			return realPath(candidate)
		}
		return ""
	}
	candidates := []string{filepath.Join(mdDir, filepath.FromSlash(normalized))}
	if strings.HasPrefix(normalized, "images/") {
		candidates = append(candidates, filepath.Join(imagesDir, filepath.FromSlash(strings.TrimPrefix(normalized, "images/"))))
	}
	candidates = append(candidates, filepath.Join(imagesDir, filepath.FromSlash(normalized)))
	for _, candidate := range candidates {
		if fileExists(candidate) {
			return realPath(candidate)
		}
	}
	return ""
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular()
}

func parseMarkdownImages(markdown string) ([]markdownImageRef, []string) {
	lines := strings.Split(markdown, "\n")
	var refs []markdownImageRef
	for lineNo, line := range lines {
		for pos := 0; pos < len(line); {
			start := strings.Index(line[pos:], "[")
			if start < 0 {
				break
			}
			start += pos
			open := strings.Index(line[start+1:], "](")
			if open < 0 {
				break
			}
			open += start + 1
			targetStart := open + 2
			if targetStart >= len(line) {
				break
			}
			targetEnd := targetStart
			if line[targetStart] == '<' {
				targetStart++
				targetEnd = strings.IndexByte(line[targetStart:], '>')
				if targetEnd < 0 {
					break
				}
				targetEnd += targetStart
			} else {
				depth := 0
				for targetEnd < len(line) {
					switch line[targetEnd] {
					case '(':
						depth++
					case ')':
						if depth == 0 {
							goto found
						}
						depth--
					}
					targetEnd++
				}
			}
		found:
			raw := strings.TrimSpace(line[targetStart:targetEnd])
			if line[open+2] != '<' {
				// CommonMark allows an optional title after the destination.
				if fields := strings.Fields(raw); len(fields) > 1 {
					raw = fields[0]
				}
			}
			isImage := start > 0 && line[start-1] == '!'
			ext := strings.ToLower(filepath.Ext(normalizeImageRef(raw)))
			isImage = isImage || ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp" || ext == ".gif" || ext == ".pdf"
			if raw != "" && isImage {
				refs = append(refs, markdownImageRef{raw: raw, decoded: normalizeImageRef(raw), line: lineNo})
			}
			pos = targetEnd + 1
		}
	}
	return refs, lines
}
