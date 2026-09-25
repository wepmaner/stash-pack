package pack

import "testing"

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"config.yaml", "config.yaml", true},
		{"config.yaml", "sub/config.yaml", false},
		{"*.md", "README.md", true},
		{"*.md", "docs/guide.md", false},
		{"**/*.md", "docs/guide.md", true},
		{"**/*.md", "README.md", true},
		{"data/**", "data/a.db", true},
		{"data/**", "data/deep/b.db", true},
		{"data/**", "data", true},
		{"data/**", "database.db", false},
		{"src/**", "src/main.go", true},
		{"*.map", "assets/app.js.map", false},
		{"**/*.map", "assets/app.js.map", true},
		{"bin/*.exe", "bin/bridge.exe", true},
		{"bin/*.exe", "bin/tools/x.exe", false},
		{".env", ".env", true},
		{"a/**/z.txt", "a/z.txt", true},
		{"a/**/z.txt", "a/b/c/z.txt", true},
		{"a/**/z.txt", "a/b/c/y.txt", false},
		{"Config.YAML", "config.yaml", true}, // Windows: регистр не важен
	}
	for _, c := range cases {
		if got := Match(c.pattern, c.path); got != c.want {
			t.Errorf("Match(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestMatchAny(t *testing.T) {
	pats := []string{"*.md", "data/**"}
	if !MatchAny(pats, "data/x") || !MatchAny(pats, "a.md") || MatchAny(pats, "a.txt") {
		t.Fatal("MatchAny работает не так")
	}
}
