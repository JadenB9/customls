package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDisplayWidth(t *testing.T) {
	cases := map[string]int{
		"":          0,
		"abc":       3,
		"café":      4, // accented letter is one column
		"日本":        4, // CJK is two columns each
		"a😀b":       4, // emoji is two columns
		"résumé.md": 9,
	}
	for s, want := range cases {
		if got := displayWidth(s); got != want {
			t.Errorf("displayWidth(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestFormatSize(t *testing.T) {
	cases := map[int64]string{
		0:             "0",
		512:           "512",
		1024:          "1.0K",
		1536:          "1.5K",
		1 << 20:       "1.0M",
		3 << 30:       "3.0G",
		1<<30 + 1<<29: "1.5G",
	}
	for n, want := range cases {
		if got := formatSize(n); got != want {
			t.Errorf("formatSize(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestColorRank(t *testing.T) {
	cfg := Config{Colors: map[string]string{"notes": "pink", "src": "purple"}}
	// Named colors sort before default dirs, which sort before plain files.
	if colorRank("notes", false, cfg) != "pink" {
		t.Error("colored file should rank by its color name")
	}
	if colorRank("lib", true, cfg) >= colorRank("readme", false, cfg) {
		t.Error("default dir should rank before a plain file")
	}
	if colorRank("src", true, cfg) >= colorRank("lib", true, cfg) {
		t.Error("colored dir should rank before a default dir")
	}
}

func TestColorize(t *testing.T) {
	cfg := Config{Colors: map[string]string{"notes": "pink", "broken": "ultraviolet"}}

	if got := colorize("notes", false, cfg, true); got != colorCodes["pink"]+"notes"+resetCode {
		t.Errorf("custom color not applied: %q", got)
	}
	if got := colorize("lib", true, cfg, true); got != defaultDirColor+"lib"+resetCode {
		t.Errorf("dir should get the default color: %q", got)
	}
	if got := colorize("plain.txt", false, cfg, true); got != "plain.txt" {
		t.Errorf("plain file should be untouched: %q", got)
	}
	// A dir whose config names a color that doesn't exist keeps the default
	// instead of losing color entirely.
	if got := colorize("broken", true, cfg, true); got != defaultDirColor+"broken"+resetCode {
		t.Errorf("unknown color should fall back to the dir default: %q", got)
	}
	// color=false means no escape codes at all, even for custom entries
	if got := colorize("notes", true, cfg, false); got != "notes" {
		t.Errorf("color off should give the bare name: %q", got)
	}
}

func TestParseArgs(t *testing.T) {
	cases := []struct {
		args []string
		cmd  string
		opts listOpts
		path string
	}{
		{nil, "", listOpts{}, ""},
		{[]string{"-a"}, "", listOpts{all: true}, ""},
		{[]string{"-l", "/tmp"}, "", listOpts{long: true}, "/tmp"},
		{[]string{"-al"}, "", listOpts{all: true, long: true}, ""},
		{[]string{"-la", "docs"}, "", listOpts{all: true, long: true}, "docs"},
		{[]string{"docs", "-l"}, "", listOpts{long: true}, "docs"},
		{[]string{"-1"}, "", listOpts{one: true}, ""},
		{[]string{"-a", "-l", "-1"}, "", listOpts{all: true, long: true, one: true}, ""},
		{[]string{"set"}, "set", listOpts{}, ""},
		{[]string{"set", "src"}, "set", listOpts{}, "src"},
		{[]string{"colors"}, "colors", listOpts{}, ""},
		{[]string{"order"}, "order", listOpts{}, ""},
		{[]string{"help"}, "help", listOpts{}, ""},
		{[]string{"--help"}, "help", listOpts{}, ""},
		{[]string{"-h"}, "help", listOpts{}, ""},
		{[]string{"version"}, "version", listOpts{}, ""},
		{[]string{"--version"}, "version", listOpts{}, ""},
		{[]string{"-"}, "", listOpts{}, "-"}, // a lone dash is a path, as in ls
	}
	for _, c := range cases {
		cmd, opts, path, err := parseArgs(c.args)
		if err != nil {
			t.Errorf("parseArgs(%v) unexpected error: %v", c.args, err)
			continue
		}
		if cmd != c.cmd || opts != c.opts || path != c.path {
			t.Errorf("parseArgs(%v) = (%q, %+v, %q), want (%q, %+v, %q)",
				c.args, cmd, opts, path, c.cmd, c.opts, c.path)
		}
	}
}

func TestParseArgsErrors(t *testing.T) {
	bad := [][]string{
		{"-x"},
		{"-lx"},
		{"a", "b"},
		{"set", "colors"},
	}
	for _, args := range bad {
		if _, _, _, err := parseArgs(args); err == nil {
			t.Errorf("parseArgs(%v) should fail", args)
		}
	}
}

// makeTree builds a directory with a known set of files and dirs.
func makeTree(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		p := filepath.Join(dir, n)
		if strings.HasSuffix(n, "/") {
			if err := os.Mkdir(strings.TrimSuffix(p, "/"), 0755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func names(items []item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.name
	}
	return out
}

func TestReadItemsHidesDotfiles(t *testing.T) {
	dir := makeTree(t, "b.txt", ".hidden", "A/", ".git/")

	items, err := readItems(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(names(items), ","); got != "A,b.txt" && got != "b.txt,A" {
		t.Errorf("without -a got %v", names(items))
	}

	items, err = readItems(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Errorf("with -a expected 4 entries, got %v", names(items))
	}
}

func TestReadItemsSingleFile(t *testing.T) {
	dir := makeTree(t, "only.txt")
	path := filepath.Join(dir, "only.txt")

	items, err := readItems(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].name != path || items[0].isDir {
		t.Errorf("listing a file should give that file back, got %+v", items)
	}

	if _, err := readItems(filepath.Join(dir, "nope"), false); err == nil {
		t.Error("missing path should error")
	}
}

func TestSortItems(t *testing.T) {
	items := []item{
		{name: "zeta"},
		{name: "Alpha"},
		{name: "beta", isDir: true},
		{name: "notes"},
	}
	sortItems(items, Config{})
	if got := strings.Join(names(items), " "); got != "Alpha beta notes zeta" {
		t.Errorf("plain sort should be case-insensitive alphabetical, got %q", got)
	}

	// With ordering on, pink "notes" comes first, then the default dir,
	// then the plain files alphabetically.
	cfg := Config{Colors: map[string]string{"notes": "pink"}, OrderByColor: true}
	sortItems(items, cfg)
	if got := strings.Join(names(items), " "); got != "notes beta Alpha zeta" {
		t.Errorf("color sort wrong, got %q", got)
	}
}

func TestPrintGridColumnMajor(t *testing.T) {
	items := []item{{name: "a"}, {name: "b"}, {name: "c"}, {name: "d"}, {name: "e"}}

	// Column width is max name + 2 = 4, so 8 columns fit in 32 -> one row.
	var buf bytes.Buffer
	printGrid(&buf, items, Config{}, 32, false)
	if got := buf.String(); got != "a   b   c   d   e\n" {
		t.Errorf("one-row grid wrong: %q", got)
	}

	// Width 8 fits two columns: fill down the first column, then the next.
	buf.Reset()
	printGrid(&buf, items, Config{}, 8, false)
	want := "a   d\nb   e\nc\n"
	if got := buf.String(); got != want {
		t.Errorf("two-column grid wrong:\n%q\nwant\n%q", got, want)
	}

	// Too narrow for even one column still prints something sensible.
	buf.Reset()
	printGrid(&buf, items, Config{}, 1, false)
	if got := buf.String(); got != "a\nb\nc\nd\ne\n" {
		t.Errorf("narrow grid wrong: %q", got)
	}

	// Empty input prints nothing.
	buf.Reset()
	printGrid(&buf, nil, Config{}, 80, false)
	if buf.Len() != 0 {
		t.Errorf("empty grid should print nothing, got %q", buf.String())
	}
}

func TestPrintGridPadsByDisplayWidth(t *testing.T) {
	// "日本" is 4 columns wide even though it's 6 bytes; padding should line
	// the second column up at the same place for both rows.
	items := []item{{name: "日本"}, {name: "abcd"}, {name: "x"}, {name: "y"}}
	var buf bytes.Buffer
	printGrid(&buf, items, Config{}, 12, false)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 rows, got %q", buf.String())
	}
	if displayWidth(lines[0]) != displayWidth(lines[1]) {
		t.Errorf("rows should be the same visual width: %q / %q", lines[0], lines[1])
	}
}

func TestPrintLines(t *testing.T) {
	cfg := Config{Colors: map[string]string{"b": "red"}}
	items := []item{{name: "a", isDir: true}, {name: "b"}}

	var buf bytes.Buffer
	printLines(&buf, items, cfg, false)
	if got := buf.String(); got != "a\nb\n" {
		t.Errorf("no-color lines wrong: %q", got)
	}

	buf.Reset()
	printLines(&buf, items, cfg, true)
	if !strings.Contains(buf.String(), colorCodes["red"]+"b"+resetCode) {
		t.Errorf("colored lines missing escape codes: %q", buf.String())
	}
}

func TestPrintLong(t *testing.T) {
	dir := makeTree(t, "small.txt", "sub/")
	big := filepath.Join(dir, "big.bin")
	if err := os.WriteFile(big, bytes.Repeat([]byte("x"), 2048), 0644); err != nil {
		t.Fatal(err)
	}
	items, err := readItems(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	sortItems(items, Config{})

	var buf bytes.Buffer
	printLong(&buf, items, Config{}, false)
	out := buf.String()

	if !strings.Contains(out, "2.0K") {
		t.Errorf("long format should show a human size, got:\n%s", out)
	}
	if !strings.Contains(out, "d") || !strings.Contains(out, "sub") {
		t.Errorf("long format should show the dir, got:\n%s", out)
	}
	year := time.Now().Format("Jan")
	if !strings.Contains(out, year) {
		t.Errorf("long format should show the mtime month, got:\n%s", out)
	}
	// Three lines, one per entry
	if n := strings.Count(out, "\n"); n != 3 {
		t.Errorf("expected 3 lines, got %d:\n%s", n, out)
	}
}

func TestShowColors(t *testing.T) {
	var buf bytes.Buffer
	showColors(&buf, Config{Colors: map[string]string{}}, false)
	if !strings.Contains(buf.String(), "No custom colors") {
		t.Errorf("empty config message missing: %q", buf.String())
	}

	buf.Reset()
	cfg := Config{Colors: map[string]string{"zz": "red", "aa": "blue", "odd": "mauve"}}
	showColors(&buf, cfg, false)
	out := buf.String()
	if strings.Index(out, "aa") > strings.Index(out, "zz") {
		t.Errorf("colors should be listed in name order:\n%s", out)
	}
	if !strings.Contains(out, "unknown color") || !strings.Contains(out, "mauve") {
		t.Errorf("unknown color name should be flagged:\n%s", out)
	}
	if strings.Contains(out, "\033[") {
		t.Errorf("color=false should print no escape codes:\n%s", out)
	}
}

func TestLoadConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgDir := filepath.Join(home, ".config", "customls")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatal(err)
	}

	// No file yet: empty config, no error
	cfg := loadConfig()
	if cfg.Colors == nil || len(cfg.Colors) != 0 || cfg.OrderByColor {
		t.Errorf("missing file should give an empty config, got %+v", cfg)
	}

	// Round trip through save
	cfg.Colors["src"] = "purple"
	cfg.OrderByColor = true
	if err := saveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	back := loadConfig()
	if back.Colors["src"] != "purple" || !back.OrderByColor {
		t.Errorf("saved config didn't load back: %+v", back)
	}

	// Broken JSON is ignored rather than crashing, and leaves an empty config
	if err := os.WriteFile(filepath.Join(cfgDir, "colors.json"), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	broken := loadConfig()
	if len(broken.Colors) != 0 || broken.OrderByColor {
		t.Errorf("broken file should give an empty config, got %+v", broken)
	}
}

func TestUseColorRespectsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if useColor() {
		t.Error("NO_COLOR set should disable color")
	}
}
