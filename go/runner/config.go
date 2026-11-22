package runner

import (
	"compare-figure/session"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

type Object = map[string]any

// LoadConfig accepts the same JSON/YAML configuration as the development prototype.
func LoadConfig(path string) (Object, error) {
	if path == "" {
		return Object{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	result := Object{}
	if ext := filepath.Ext(path); ext == ".yaml" || ext == ".yml" {
		err = yaml.Unmarshal(data, &result)
	} else {
		err = json.Unmarshal(data, &result)
	}
	if err != nil {
		return nil, fmt.Errorf("configuration: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("configuration must be an object")
	}
	if len(Obj(result, "models")) > 0 {
		var node yaml.Node
		if yaml.Unmarshal(data, &node) == nil && len(node.Content) > 0 {
			top := node.Content[0]
			for i := 0; i+1 < len(top.Content); i += 2 {
				if top.Content[i].Value == "models" {
					order := []string{}
					models := top.Content[i+1]
					for j := 0; j+1 < len(models.Content); j += 2 {
						order = append(order, models.Content[j].Value)
					}
					result["_model_order"] = order
					break
				}
			}
		}
	}
	return result, nil
}
func Clone(m Object) Object {
	if m == nil {
		return Object{}
	}
	data, _ := json.Marshal(m)
	var out Object
	_ = json.Unmarshal(data, &out)
	return out
}
func Merge(base, overrides Object) Object {
	out := Clone(base)
	for key, value := range overrides {
		if value == nil {
			delete(out, key)
			continue
		}
		if child, ok := value.(map[string]any); ok {
			if before, ok := out[key].(map[string]any); ok {
				out[key] = Merge(before, child)
				continue
			}
		}
		out[key] = value
	}
	return out
}
func Obj(m Object, key string) Object {
	if value, ok := m[key].(map[string]any); ok && value != nil {
		return value
	}
	return Object{}
}
func Str(m Object, key, defaultValue string) string {
	if value, ok := m[key].(string); ok && value != "" {
		return value
	}
	return defaultValue
}
func Num(m Object, key string, defaultValue float64) float64 {
	value, ok := m[key]
	if !ok || value == nil {
		return defaultValue
	}
	switch n := value.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		v, e := n.Float64()
		if e == nil {
			return v
		}
	case string:
		v, e := strconv.ParseFloat(n, 64)
		if e == nil {
			return v
		}
	}
	return defaultValue
}
func Bool(m Object, key string, defaultValue bool) bool {
	if b, ok := m[key].(bool); ok {
		return b
	}
	return defaultValue
}
func nonzero(m Object, key string, defaultValue float64) float64 {
	n := Num(m, key, defaultValue)
	if n == 0 {
		return defaultValue
	}
	return n
}

func ModelConfig(config Object, name string) (Object, error) {
	registry := Obj(config, "models")
	if len(registry) == 0 {
		out := Clone(config)
		for _, key := range []string{"session", "checker_session", "figure_check", "view", "compile"} {
			delete(out, key)
		}
		return out, nil
	}
	if name == "" {
		name = Str(Obj(config, "latex"), "drawing_model", "drawing")
	}
	if _, ok := registry[name]; !ok {
		order := []string{}
		switch saved := config["_model_order"].(type) {
		case []string:
			order = append(order, saved...)
		case []any:
			for _, value := range saved {
				if alias, ok := value.(string); ok {
					order = append(order, alias)
				}
			}
		}
		if len(order) == 0 {
			for alias := range registry {
				order = append(order, alias)
			}
			sort.Strings(order)
		}
		for _, alias := range order {
			value := registry[alias]
			if entry, ok := value.(map[string]any); ok && Str(entry, "model", "") == name {
				name = alias
				break
			}
		}
	}
	var expand func(string, map[string]bool) (Object, error)
	expand = func(alias string, seen map[string]bool) (Object, error) {
		if seen[alias] {
			return nil, fmt.Errorf("model extends cycle: %s", alias)
		}
		entry, ok := registry[alias].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("model alias not configured: %s", alias)
		}
		seen[alias] = true
		defer delete(seen, alias)
		if parent := Str(entry, "extends", ""); parent != "" {
			base, err := expand(parent, seen)
			if err != nil {
				return nil, err
			}
			return Merge(base, entry), nil
		}
		return Clone(entry), nil
	}
	entry, err := expand(name, map[string]bool{})
	if err != nil {
		return nil, err
	}
	if name != "text" {
		if _, ok := registry["text"]; ok {
			base, e := expand("text", map[string]bool{})
			if e != nil {
				return nil, e
			}
			for _, key := range []string{"max_images", "context_limit", "max_concurrency"} {
				delete(base, key)
			}
			entry = Merge(base, entry)
		}
	}
	return entry, nil
}

func MakeClient(config Object, name string, overrides Object, timeout time.Duration) (*session.Client, error) {
	entry, err := ModelConfig(config, name)
	if err != nil {
		return nil, err
	}
	entry = Merge(entry, overrides)
	if name != "" && len(Obj(config, "models")) == 0 {
		entry["model"] = name
	}
	model := Str(entry, "model", os.Getenv("OPENAI_MODEL"))
	base := Str(entry, "base_url", os.Getenv("OPENAI_BASE_URL"))
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	keyEnv := Str(entry, "api_key_env", "OPENAI_API_KEY")
	key := Str(entry, "api_key", os.Getenv(keyEnv))
	if model == "" || key == "" {
		return nil, fmt.Errorf("configure a model and API key (environment variable %s)", keyEnv)
	}
	if value := Num(entry, "api_timeout", 0); value > 0 {
		timeout = time.Duration(value * float64(time.Second))
	}
	for _, key := range []string{"model", "base_url", "api_key", "api_key_env", "api_timeout"} {
		delete(entry, key)
	}
	return session.NewClient(base, model, key, timeout, entry), nil
}

func StageSettings(config Object) (tuning, checker, checks, view, compile Object) {
	latex := Obj(config, "latex")
	sessions := Obj(latex, "sessions")
	tuning = Obj(config, "session")
	if _, ok := config["session"]; !ok {
		tuning = Obj(sessions, "drawing")
	}
	checker = Obj(config, "checker_session")
	if _, ok := config["checker_session"]; !ok {
		checker = Obj(sessions, "checker")
	}
	checks = Obj(config, "figure_check")
	if _, ok := config["figure_check"]; !ok {
		checks = Obj(latex, "figure_check")
	}
	view = Obj(config, "view")
	if _, ok := config["view"]; !ok {
		view = Obj(Obj(config, "tools"), "view")
	}
	compile = Obj(config, "compile")
	if _, ok := config["compile"]; !ok {
		compile = Obj(latex, "compile")
	}
	return Clone(tuning), Clone(checker), Clone(checks), Clone(view), Clone(compile)
}
