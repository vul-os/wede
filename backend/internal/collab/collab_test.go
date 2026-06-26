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

func TestTagRelay(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"mouse", `{"type":"mouse","x":0.5,"y":0.25}`, true},
		{"window", `{"type":"window","win":"term-1","geo":{"x":10,"y":20}}`, true},
		{"cursor not relayed", `{"type":"cursor","file":"a"}`, false},
		{"unknown type", `{"type":"chat","text":"hi"}`, false},
		{"no type", `{"x":1}`, false},
		{"malformed", `{nope`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, ok := tagRelay("m7", []byte(c.in))
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if ok && !contains(string(out), `"id":"m7"`) {
				t.Fatalf("relayed message missing sender id: %s", out)
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
