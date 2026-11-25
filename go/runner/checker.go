package runner

import (
	"compare-figure/drawing"
	"compare-figure/render"
	"compare-figure/session"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type checkerTools struct{ state Object }

func (c *checkerTools) Definitions() []map[string]any {
	return []map[string]any{{"type": "function", "function": Object{"name": "submit", "description": "Declare the comparison of the redrawn figure with the original final. Only call after a clean compile.", "parameters": Object{"type": "object", "properties": Object{"notes": Object{"type": "string", "description": "Optional short handover notes."}, "report": Object{"type": "object", "description": "REQUIRED verdict. status=pass when the redrawn figure faithfully reproduces the original's content and layout; status=issues when it does not, with report.issues listing each visible discrepancy (what is wrong and where).", "properties": Object{"status": Object{"type": "string", "enum": []string{"pass", "issues"}}, "issues": Object{"type": "string"}, "suggestions": Object{"type": "string"}}, "required": []string{"status"}}}}}}}
}
func (c *checkerTools) Execute(ctx context.Context, name string, args map[string]any) (string, string, error) {
	if name != "submit" {
		return "TOOL ERROR: unknown tool " + name, "", nil
	}
	report, ok := args["report"].(map[string]any)
	if !ok {
		return "REJECTED: submit requires a 'report' object (work report / 工作汇报). Call submit again with report.status = pass or issues.", "", nil
	}
	c.state = Merge(c.state, report)
	c.state["submitted"] = true
	return "SUBMITTED.", "", nil
}
func (c *checkerTools) Snapshot() map[string]any { return Clone(c.state) }

func rasterize(ctx context.Context, pdf, prefix string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(prefix), 0755); err != nil {
		return "", err
	}
	sub, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(sub, "pdftoppm", "-png", "-singlefile", "-f", "1", "-l", "1", "-r", "110", pdf, prefix).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("PDF preview: %w: %s", err, string(output))
	}
	return prefix + ".png", nil
}

// VerifyFigure retains the accepted state if the checker or a revision fails.
func VerifyFigure(ctx context.Context, h *drawing.Harness, drawingSession *session.Session, original string, client session.Completer, tuning, checks Object, language, resources string) []string {
	warnings := []string{}
	maximum := max(1, int(Num(checks, "max_rounds", 2)))
	root := h.Root
	checkerSystem, err := Prompt(resources, "latex_figurecheck.system.md", Object{"MAX_ROUNDS": Num(tuning, "max_tool_rounds", 50), "OUTPUT_LANG": language})
	if err != nil {
		return []string{"Figure check unavailable; delivering current version: " + err.Error()}
	}
	transcript, err := session.NewTranscript(filepath.Join(root, "figure_check.jsonl"))
	if err != nil {
		return []string{"Figure check unavailable; delivering current version: " + err.Error()}
	}
	tools := &checkerTools{Object{"submitted": false}}
	checker := session.NewSession(tools, client, transcript, checkerSystem, tuning, nil)
	for round := 1; round <= maximum; round++ {
		tools.state["submitted"] = false
		png, e := h.Rasterize(ctx, Str(h.Snapshot(), "last_pdf", ""), filepath.Join(root, "预览", fmt.Sprintf("check_%d", round)))
		if e != nil {
			warnings = append(warnings, "Figure check unavailable; delivering current version: "+e.Error())
			break
		}
		user, e := Prompt(resources, "latex_figurecheck.user.md", Object{"ORIGINAL": filepath.Base(original), "REDRAWN": filepath.Base(png)})
		if e != nil {
			warnings = append(warnings, e.Error())
			break
		}
		if round > 1 {
			user = fmt.Sprintf("This is round %d of the same comparison, after the drawing session tried to fix the problems you listed. Compare the two images again and submit your verdict.", round)
		}
		originalPart, e := session.ImagePart(original, 1280)
		if e != nil {
			warnings = append(warnings, e.Error())
			break
		}
		redrawnPart, e := session.ImagePart(png, 1280)
		if e != nil {
			warnings = append(warnings, e.Error())
			break
		}
		if e = checker.Record(session.Message{"role": "user", "content": []any{Object{"type": "text", "text": user}, originalPart, redrawnPart}}); e != nil {
			warnings = append(warnings, e.Error())
			break
		}
		if _, e = checker.Run(ctx, "", nil); e != nil {
			warnings = append(warnings, "Figure check unavailable; delivering current version: "+e.Error())
			break
		}
		if !Bool(tools.state, "submitted", false) {
			warnings = append(warnings, "Figure checker did not submit a verdict; delivering current version")
			break
		}
		if Str(tools.state, "status", "") != "issues" {
			break
		}
		problems := Str(tools.state, "issues", "the checker reported visible differences but gave no description")
		if round >= maximum {
			warnings = append(warnings, "Figure check round limit reached: "+problems)
			break
		}
		accepted := h.Snapshot()
		pending := Clone(accepted)
		pending["submitted"] = false
		if e = h.Restore(pending); e != nil {
			warnings = append(warnings, e.Error())
			break
		}
		feedback := strings.ReplaceAll(session.CheckFeedback, "{problems}", problems)
		mode := Str(h.Options, "drawing_mode", "luadraw")
		language := Str(accepted, "source_language", "")
		if language == "" {
			language = Str(accepted, "drawing_backend", mode)
		}
		feedback = strings.ReplaceAll(feedback, "figure.tex", Str(accepted, "source_path", render.DefaultSource(mode)))
		feedback += fmt.Sprintf("\nContinue within drawing mode %s using the accepted source language %s. Policy: %s. Use document_search for relevant reference material.", mode, language, render.SourceDescription(mode))
		_, e = drawingSession.Run(ctx, feedback, nil)
		if e != nil || !h.IsSubmitted() {
			_ = h.Restore(accepted)
			if e != nil {
				warnings = append(warnings, "Figure correction failed; delivering current version: "+e.Error())
			} else {
				warnings = append(warnings, "Drawing correction did not submit an accepted revision; delivering previous accepted version")
			}
			break
		}
		if Str(h.Snapshot(), "final_code", "") == Str(accepted, "final_code", "") {
			warnings = append(warnings, "Drawing session made no change after figure check")
			break
		}
	}
	return warnings
}
