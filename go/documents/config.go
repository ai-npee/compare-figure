// Package documents adapts the pinned doc-library dependency to drawing tools.
// Indexing, extraction, navigation, paragraph identity and frequency are owned by
// doc-library; this package only selects bundled sources and formats tool output.
package documents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"doc-library/pkg/doclibrary"
	"gopkg.in/yaml.v3"
)

const DependencyRevision = "ce677c8064e71413f97fa7f8467ec5df6bc8a775"

type OutputSettings = doclibrary.OutputSettings

type Config = doclibrary.Config
type Source = doclibrary.Source
type Request = doclibrary.Request
type Response = doclibrary.Response

type Docs struct {
	library      *doclibrary.Library
	emptyMessage string
	output       OutputSettings
}

// New selects a backend's docs/document-sources/<mode>.yaml when present.
// Source roots and a configured relative cache_dir resolve from that YAML file;
// cacheDir, when supplied, overrides its cache setting. Without a configuration,
// the original official references (or an empty native LaTeX collection) apply.
// rootResource may be the Skill root or its legacy vendor/luadraw-3.7/luadraw root.
func New(rootResource, cacheDir, mode string) (*Docs, error) {
	return NewWithOutput(rootResource, cacheDir, mode, OutputSettings{})
}

// NewWithOutput applies startup presentation overrides after source YAML. Zero
// version and nil IncludeBBox inherit; source selection is unchanged. New and
// this constructor default an unspecified output version to host protocol 2.
func NewWithOutput(rootResource, cacheDir, mode string, output OutputSettings) (*Docs, error) {
	if err := validateOutput(output); err != nil {
		return nil, err
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "luadraw"
	}
	if mode != "luadraw" && mode != "mixed" && mode != "latex" && mode != "asymptote" {
		return nil, fmt.Errorf("unknown document backend %q", mode)
	}
	if rootResource == "" {
		if mode == "latex" {
			return emptyLatexDocs(output), nil
		}
		return nil, fmt.Errorf("document resource root is required")
	}
	root, err := filepath.Abs(rootResource)
	if err != nil {
		return nil, err
	}
	resource := root
	// Only the canonical bundled legacy location identifies a Skill root.
	// Independent legacy collections must not acquire ancestor/cwd configs.
	if isDir(filepath.Join(root, "doc")) {
		resource = ""
		candidate := filepath.Clean(filepath.Join(root, "..", "..", ".."))
		if filepath.Join(candidate, "vendor", "luadraw-3.7", "luadraw") == root {
			resource = candidate
		}
	}
	if resource != "" {
		path := filepath.Join(resource, "docs", "document-sources", mode+".yaml")
		docs, found, err := configuredDocs(path, cacheDir, output)
		if err != nil || found {
			return docs, err
		}
	}
	if mode == "latex" {
		return emptyLatexDocs(output), nil
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
		if resource == "" {
			return nil, fmt.Errorf("Asymptote documentation requires a bundled resource root")
		}
		asy, err := asymptoteRoot(resource)
		if err != nil {
			return nil, err
		}
		sources = append(sources, Source{ID: "asymptote", Root: asy})
	}
	return NewConfig(Config{Sources: sources, CacheDir: cacheDir, Output: hostOutput(OutputSettings{}, output)})
}

func emptyLatexDocs(output OutputSettings) *Docs {
	return &Docs{output: hostOutput(OutputSettings{}, output), emptyMessage: "No bundled reference for native LaTeX; configure document sources to add a reference collection."}
}

// configuredDocs reads one standard doc-library Config, without overlays or
// source-specific policies. Only a missing file permits the official fallback.
func configuredDocs(path, cacheDir string, output OutputSettings) (*Docs, bool, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, fmt.Errorf("document sources config %s: %w", path, err)
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	var config Config
	if err = decoder.Decode(&config); err != nil {
		return nil, true, fmt.Errorf("document sources config %s: %w", path, err)
	}
	var extra yaml.Node
	if err = decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("expected exactly one YAML document")
		}
		return nil, true, fmt.Errorf("document sources config %s: %w", path, err)
	}
	base := filepath.Dir(path)
	for i := range config.Sources {
		root := config.Sources[i].Root
		if root != "" && !filepath.IsAbs(root) {
			config.Sources[i].Root = filepath.Join(base, filepath.FromSlash(root))
		}
	}
	if config.CacheDir != "" && !filepath.IsAbs(config.CacheDir) {
		config.CacheDir = filepath.Join(base, filepath.FromSlash(config.CacheDir))
	}
	if cacheDir != "" {
		config.CacheDir = cacheDir
	}
	config.Output = hostOutput(config.Output, output)
	docs, err := NewConfig(config)
	if err != nil {
		return nil, true, fmt.Errorf("document sources config %s: %w", path, err)
	}
	return docs, true, nil
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
	return &Docs{library: library, output: config.Output}, nil
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

// OutputFromOptions decodes public options.doc_output startup settings, retaining
// explicit false and rejecting misspelled fields or invalid protocol versions.
func OutputFromOptions(options map[string]any) (OutputSettings, error) {
	var output OutputSettings
	raw, exists := options["doc_output"]
	if !exists || raw == nil {
		return output, nil
	}
	if _, ok := raw.(map[string]any); !ok {
		return output, fmt.Errorf("options.doc_output must be an object")
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return output, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&output); err != nil {
		return output, fmt.Errorf("options.doc_output: %w", err)
	}
	return output, validateOutput(output)
}
func validateOutput(output OutputSettings) error {
	if output.Version != 0 && output.Version != 1 && output.Version != 2 {
		return fmt.Errorf("document output version must be 1 or 2")
	}
	return nil
}
func hostOutput(config, override OutputSettings) OutputSettings {
	if config.Version == 0 {
		config.Version = 2
	}
	if override.Version != 0 {
		config.Version = override.Version
	}
	if override.IncludeBBox != nil {
		value := *override.IncludeBBox
		config.IncludeBBox = &value
	}
	return config
}
