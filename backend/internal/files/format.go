package files

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
)

// prettierParser maps a file extension to the --parser value prettier expects.
var prettierParser = map[string]string{
	".js":   "babel",
	".jsx":  "babel",
	".ts":   "typescript",
	".tsx":  "typescript",
	".css":  "css",
	".json": "json",
	".html": "html",
	".md":   "markdown",
}

// Format formats source code using the appropriate formatter for the file
// extension, returning the formatted content.  If no formatter is available
// or the content cannot be formatted, the original content is returned with
// formatted=false.
//
// POST /api/files/format  {"path": "src/foo.go", "content": "..."}
func (h *Handler) Format(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "path and content required"})
		return
	}

	ext := strings.ToLower(filepath.Ext(body.Path))
	base := filepath.Base(body.Path)

	type result struct {
		Content   string `json:"content"`
		Formatted bool   `json:"formatted"`
		Error     string `json:"error,omitempty"`
	}

	noOp := func(errMsg string) {
		json.NewEncoder(w).Encode(result{Content: body.Content, Formatted: false, Error: errMsg})
	}

	switch ext {
	case ".go":
		bin, err := exec.LookPath("gofmt")
		if err != nil {
			noOp("formatter not available")
			return
		}
		cmd := exec.Command(bin)
		cmd.Stdin = strings.NewReader(body.Content)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			noOp(strings.TrimSpace(stderr.String()))
			return
		}
		json.NewEncoder(w).Encode(result{Content: stdout.String(), Formatted: true})

	case ".js", ".jsx", ".ts", ".tsx", ".css", ".json", ".html", ".md":
		parser := prettierParser[ext]
		bin, err := exec.LookPath("prettier")
		if err != nil {
			noOp("formatter not available")
			return
		}
		cmd := exec.Command(bin, "--parser", parser, "--stdin-filepath", base)
		cmd.Stdin = strings.NewReader(body.Content)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			noOp(strings.TrimSpace(stderr.String()))
			return
		}
		json.NewEncoder(w).Encode(result{Content: stdout.String(), Formatted: true})

	case ".py":
		bin, err := exec.LookPath("black")
		if err != nil {
			noOp("formatter not available")
			return
		}
		cmd := exec.Command(bin, "-q", "-")
		cmd.Stdin = strings.NewReader(body.Content)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			noOp(strings.TrimSpace(stderr.String()))
			return
		}
		json.NewEncoder(w).Encode(result{Content: stdout.String(), Formatted: true})

	default:
		noOp("")
	}
}
