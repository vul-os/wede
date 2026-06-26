package collab

import "testing"

func TestParseCursor(t *testing.T) {
	cases := []struct {
		name string
		in   string
		file string
		line int
		ok   bool
	}{
		{"full", `{"type":"cursor","file":"src/a.go","line":42}`, "src/a.go", 42, true},
		{"no line", `{"type":"cursor","file":"x"}`, "x", 0, true},
		{"wrong type", `{"type":"presence"}`, "", 0, false},
		{"ping type", `{"type":"ping"}`, "", 0, false},
		{"malformed", `not-json`, "", 0, false},
		{"empty", ``, "", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			file, line, ok := parseCursor([]byte(c.in))
			if ok != c.ok || file != c.file || line != c.line {
				t.Errorf("parseCursor(%q) = (%q, %d, %v), want (%q, %d, %v)",
					c.in, file, line, ok, c.file, c.line, c.ok)
			}
		})
	}
}
