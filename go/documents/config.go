// Package documents adapts the pinned doc-library dependency to drawing tools.
// Indexing, extraction, navigation, paragraph identity and frequency are owned by
// doc-library; this package only selects bundled sources and formats tool output.
package documents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"doc-library/pkg/doclibrary"
)

const DependencyRevision = "a705bca4420cd2126b6072a1c7b6450f2b9cc84c"

type Config = doclibrary.Config
type Source = doclibrary.Source
type Request = doclibrary.Request
type Response = doclibrary.Response

type Docs struct {
	library      *doclibrary.Library
	emptyMessage string
}

// New selects the references for a drawing backend. rootResource may be the
// Skill resource root or the legacy vendor/luadraw-3.7/luadraw root.
// Native LaTeX has no bundled reference collection; use NewConfig to add one.
func New(rootResource, cacheDir, mode string) (*Docs, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "luadraw"
	}
	if mode != "luadraw" && mode != "mixed" && mode != "latex" && mode != "asymptote" {
		return nil, fmt.Errorf("unknown document backend %q", mode)
	}
	if mode == "latex" {
		return &Docs{emptyMessage: "No bundled reference for native LaTeX; configure document sources to add a reference collection."}, nil
	}
	root, err := filepath.Abs(rootResource)
	if err != nil {
		return nil, err
	}
	if rootResource == "" {
		return nil, fmt.Errorf("document resource root is required")
	}
	resource := root
	// A direct LuaDraw root is accepted by the legacy NewDocs API.
	if isDir(filepath.Join(root, "doc")) && isDir(filepath.Join(root, "files")) {
		resource = filepath.Clean(filepath.Join(root, "..", "..", ".."))
	}
	var sources []Source
	if mode == "luadraw" || mode == "mixed" {
		lua := filepath.Join(resource, "vendor", "luadraw-3.7", "luadraw")
		if isDir(filepath.Join(root, "doc")) {
			lua = root
		}
		if !isDir(lua) {
			return nil, fmt.Errorf("LuaDraw documentation root not found: %s", lua)
		}
		sources = append(sources, Source{ID: "luadraw", Root: lua})
	}
	if mode == "asymptote" || mode == "mixed" {
		asy, err := asymptoteRoot(resource)
		if err != nil {
			return nil, err
		}
		sources = append(sources, Source{ID: "asymptote", Root: asy})
	}
	return NewConfig(Config{Sources: sources, CacheDir: cacheDir})
}

// NewConfig accepts explicit sources, filters, quotas, Poppler paths and cache
// settings using the public doc-library configuration contract.
func NewConfig(config Config) (*Docs, error) {
	library, err := doclibrary.New(config)
	if err != nil {
		return nil, err
	}
	if len(config.Sources) == 0 {
		return nil, fmt.Errorf("at least one document source is required")
	}
	return &Docs{library: library}, nil
}

func isDir(path string) bool { st, e := os.Stat(path); return e == nil && st.IsDir() }

func asymptoteRoot(resource string) (string, error) {
	// The manifest root is relative to the portable Skill root, never to cwd.
	manifest := filepath.Join(resource, "docs", "asymptote-upstream.json")
	if data, err := os.ReadFile(manifest); err == nil {
		var m struct {
			ResourceRoot string `json:"resource_root"`
			VendorRoot   string `json:"vendor_root"`
			Root         string `json:"root"`
		}
		if err = json.Unmarshal(data, &m); err != nil {
			return "", fmt.Errorf("Asymptote source manifest: %w", err)
		}
		declared := m.ResourceRoot
		if declared == "" {
			declared = m.VendorRoot
		}
		if declared == "" {
			declared = m.Root
		}
		if declared != "" {
			if filepath.IsAbs(declared) || strings.Contains(declared, "\\") {
				return "", fmt.Errorf("Asymptote manifest root must be resource-relative")
			}
			path := filepath.Join(resource, filepath.FromSlash(declared))
			rel, err := filepath.Rel(resource, path)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return "", fmt.Errorf("Asymptote manifest root escapes resources")
			}
			if !isDir(path) {
				return "", fmt.Errorf("Asymptote documentation root not found: %s", path)
			}
			return path, nil
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	matches, err := filepath.Glob(filepath.Join(resource, "vendor", "asymptote-*"))
	if err != nil {
		return "", err
	}
	var roots []string
	for _, p := range matches {
		if isDir(p) {
			roots = append(roots, p)
		}
	}
	sort.Strings(roots)
	if len(roots) != 1 {
		return "", fmt.Errorf("expected one bundled vendor/asymptote-* reference root, found %d; set docs/asymptote-upstream.json resource_root", len(roots))
	}
	return roots[0], nil
}
