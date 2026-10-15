package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Filled in by goreleaser (-X main.version=...). Stays "dev" for go build.
var version = "dev"

const resetCode = "\033[0m"
const defaultDirColor = "\033[1;36m" // cyan for dirs

var colorOptions = []struct {
	name string
	code string
}{
	{"red", "\033[1;31m"},
	{"green", "\033[1;32m"},
	{"yellow", "\033[1;33m"},
	{"blue", "\033[1;34m"},
	{"magenta", "\033[1;35m"},
	{"cyan", "\033[1;36m"},
	{"white", "\033[1;37m"},
	{"orange", "\033[38;5;208m"},
	{"pink", "\033[38;5;205m"},
	{"purple", "\033[38;5;141m"},
}

var colorCodes map[string]string

func init() {
	colorCodes = make(map[string]string)
	for _, c := range colorOptions {
		colorCodes[c.name] = c.code
	}
}

type Config struct {
	Colors       map[string]string `json:"colors"`
	OrderByColor bool              `json:"order_by_color"`
}

// colorNames is the list of valid names, in the order they're shown.
func colorNames() []string {
	names := make([]string, len(colorOptions))
	for i, c := range colorOptions {
		names[i] = c.name
	}
	return names
}

// colorRank returns a sort key: colored items get their color name (so same
// colors group together), default-colored dirs get "~dir", plain files get
// "~~file". The tilde prefixes push them after named colors alphabetically.
func colorRank(name string, isDir bool, cfg Config) string {
	if c, ok := cfg.Colors[name]; ok {
		return c
	}
	if isDir {
		return "~dir"
	}
	return "~~file"
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "customls", "colors.json")
}

// loadConfig reads colors.json. A missing file just means nothing is set yet.
// A file that won't parse gets a warning on stderr instead of silently losing
// every color, which is what used to happen after a typo.
func loadConfig() Config {
	empty := Config{Colors: make(map[string]string)}
	data, err := os.ReadFile(configPath())
	if err != nil {
		return empty
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "customls: ignoring %s: %v\n", configPath(), err)
		return empty
	}
	if cfg.Colors == nil {
		cfg.Colors = make(map[string]string)
	}
	return cfg
}

func saveConfig(cfg Config) error {
	path := configPath()
	os.MkdirAll(filepath.Dir(path), 0755)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// isTerminal reports whether f is attached to a terminal rather than a pipe
// or a file.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// useColor follows the usual rules: no escape codes when piped somewhere, and
// none when NO_COLOR is set (https://no-color.org).
func useColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return isTerminal(os.Stdout)
}

func getTermWidth() int {
	cmd := exec.Command("tput", "cols")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return 80
	}
	w, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 80
	}
	return w
}

// displayWidth counts terminal columns rather than bytes. Accented letters
// are multi-byte but one column; CJK and emoji runes draw as two columns.
// This is the usual wcwidth approximation, close enough for a file listing.
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		switch {
		case (r >= 0x1100 && r <= 0x115F) || // Hangul Jamo
			(r >= 0x2E80 && r <= 0xA4CF) || // CJK radicals through Yi
			(r >= 0xAC00 && r <= 0xD7A3) || // Hangul syllables
			(r >= 0xF900 && r <= 0xFAFF) || // CJK compatibility ideographs
			(r >= 0xFE30 && r <= 0xFE6F) || // CJK compatibility forms
			(r >= 0xFF00 && r <= 0xFF60) || // fullwidth forms
			(r >= 0xFFE0 && r <= 0xFFE6) ||
			(r >= 0x1F300 && r <= 0x1FAFF) || // emoji
			(r >= 0x20000 && r <= 0x2FFFD): // CJK extension B+
			w += 2
		default:
			w++
		}
	}
	return w
}

// colorize wraps a name in its escape code. An unknown color name in the
// config falls back to the default, so a typo doesn't make a dir go plain.
func colorize(name string, isDir bool, cfg Config, color bool) string {
	if !color {
		return name
	}
	if custom, ok := cfg.Colors[name]; ok {
		if code, ok2 := colorCodes[custom]; ok2 {
			return code + name + resetCode
		}
	}
	if isDir {
		return defaultDirColor + name + resetCode
	}
	return name
}

type item struct {
	name  string
	isDir bool
	info  os.FileInfo
}

// readItems lists what's at path. A directory gives its entries; a plain file
// gives just that file, the same way ls does.
func readItems(path string, showAll bool) ([]item, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return []item{{name: path, isDir: false, info: st}}, nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var items []item
	for _, e := range entries {
		if !showAll && strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, item{name: e.Name(), isDir: e.IsDir(), info: info})
	}
	return items, nil
}

// sortItems orders case-insensitively, grouping by color first when that's on.
func sortItems(items []item, cfg Config) {
	byName := func(i, j int) bool {
		return strings.ToLower(items[i].name) < strings.ToLower(items[j].name)
	}
	if !cfg.OrderByColor {
		sort.Slice(items, byName)
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		ri, rj := colorRank(items[i].name, items[i].isDir, cfg), colorRank(items[j].name, items[j].isDir, cfg)
		if ri != rj {
			return ri < rj
		}
		return byName(i, j)
	})
}

// printGrid lays items out column-major: fill top-to-bottom, then the next
// column, so reading down keeps alphabetical order.
func printGrid(w io.Writer, items []item, cfg Config, termWidth int, color bool) {
	if len(items) == 0 {
		return
	}

	maxLen := 0
	for _, it := range items {
		if dw := displayWidth(it.name); dw > maxLen {
			maxLen = dw
		}
	}

	colWidth := maxLen + 2
	if colWidth < 4 {
		colWidth = 4
	}
	numCols := termWidth / colWidth
	if numCols < 1 {
		numCols = 1
	}
	numRows := (len(items) + numCols - 1) / numCols

	for row := 0; row < numRows; row++ {
		for col := 0; col < numCols; col++ {
			idx := col*numRows + row
			if idx >= len(items) {
				break
			}
			it := items[idx]
			display := colorize(it.name, it.isDir, cfg, color)

			isLast := col == numCols-1 || (col+1)*numRows+row >= len(items)
			if isLast {
				fmt.Fprintln(w, display)
			} else {
				padding := colWidth - displayWidth(it.name)
				if padding < 1 {
					padding = 1
				}
				fmt.Fprint(w, display+strings.Repeat(" ", padding))
			}
		}
	}
}

// printLines is one name per line, for -1 and for piped output.
func printLines(w io.Writer, items []item, cfg Config, color bool) {
	for _, it := range items {
		fmt.Fprintln(w, colorize(it.name, it.isDir, cfg, color))
	}
}

// printLong is the -l format: permissions, size, mtime, name.
func printLong(w io.Writer, items []item, cfg Config, color bool) {
	maxSize := 0
	for _, it := range items {
		if s := len(formatSize(it.info.Size())); s > maxSize {
			maxSize = s
		}
	}
	for _, it := range items {
		perm := it.info.Mode().String()
		size := formatSize(it.info.Size())
		mod := it.info.ModTime().Format("Jan _2 15:04")
		display := colorize(it.name, it.isDir, cfg, color)
		fmt.Fprintf(w, "%s  %*s  %s  %s\n", perm, maxSize, size, mod, display)
	}
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(bytes)/float64(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%d", bytes)
	}
}

// listOpts is what the flags turn into.
type listOpts struct {
	all  bool // -a  include dotfiles
	long bool // -l  long format
	one  bool // -1  one per line
}

// list prints path with the given options to w. Piped output gets one name
// per line with no color, like ls, so `customls | grep` behaves.
func list(w io.Writer, path string, cfg Config, opts listOpts, color bool, termWidth int) error {
	items, err := readItems(path, opts.all)
	if err != nil {
		return err
	}
	sortItems(items, cfg)

	switch {
	case opts.long:
		printLong(w, items, cfg, color)
	case opts.one || !isTerminal(os.Stdout):
		printLines(w, items, cfg, color)
	default:
		printGrid(w, items, cfg, termWidth, color)
	}
	return nil
}

func interactiveSet(dir string, cfg Config) Config {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "customls: %v\n", err)
		os.Exit(1)
	}

	var items []item
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			items = append(items, item{name: e.Name(), isDir: e.IsDir()})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].name) < strings.ToLower(items[j].name)
	})

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("\n  Select an item to colorize:\n\n")
		for i, it := range items {
			marker := "  "
			if color, ok := cfg.Colors[it.name]; ok {
				if code, ok2 := colorCodes[color]; ok2 {
					marker = code + "● " + resetCode
				}
			}
			label := it.name
			if it.isDir {
				label += "/"
			}
			fmt.Printf("  %s%2d) %s\n", marker, i+1, label)
		}

		fmt.Print("\n  Enter number (q to quit): ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "q" || input == "Q" || input == "" {
			return cfg
		}

		idx, err := strconv.Atoi(input)
		if err != nil || idx < 1 || idx > len(items) {
			fmt.Println("  Invalid selection.")
			continue
		}

		selected := items[idx-1].name

		fmt.Printf("\n  Pick a color for '%s':\n\n", selected)
		for i, c := range colorOptions {
			fmt.Printf("  %s%2d) %s%s\n", c.code, i+1, c.name, resetCode)
		}
		fmt.Printf("  %2d) remove color\n", len(colorOptions)+1)

		fmt.Print("\n  Enter color number: ")
		input2, _ := reader.ReadString('\n')
		input2 = strings.TrimSpace(input2)
		cidx, err := strconv.Atoi(input2)
		if err != nil || cidx < 1 || cidx > len(colorOptions)+1 {
			fmt.Println("  Invalid selection.")
			continue
		}

		if cidx == len(colorOptions)+1 {
			delete(cfg.Colors, selected)
			fmt.Printf("\n  Removed color for '%s'\n", selected)
		} else {
			chosen := colorOptions[cidx-1]
			cfg.Colors[selected] = chosen.name
			fmt.Printf("\n  Set '%s' to %s%s%s\n", selected, chosen.code, chosen.name, resetCode)
		}

		if err := saveConfig(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "  Error saving config: %v\n", err)
		}
	}
}

// showColors prints the color table in name order. Entries that name a color
// that doesn't exist are flagged, since the only other hint is the dir
// quietly losing its color.
func showColors(w io.Writer, cfg Config, color bool) {
	if len(cfg.Colors) == 0 {
		fmt.Fprintln(w, "  No custom colors set. Run 'customls set' to add some.")
		return
	}
	names := make([]string, 0, len(cfg.Colors))
	for name := range cfg.Colors {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Fprintln(w, "\n  Custom colors:")
	for _, name := range names {
		c := cfg.Colors[name]
		code, known := colorCodes[c]
		if !known {
			fmt.Fprintf(w, "  %-20s  %s  (unknown color, pick from: %s)\n", name, c, strings.Join(colorNames(), ", "))
			continue
		}
		if color {
			fmt.Fprintf(w, "  %s%-20s%s  %s\n", code, name, resetCode, c)
		} else {
			fmt.Fprintf(w, "  %-20s  %s\n", name, c)
		}
	}
	fmt.Fprintln(w)
}

func toggleOrder(cfg Config) Config {
	cfg.OrderByColor = !cfg.OrderByColor
	if err := saveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "  Error saving config: %v\n", err)
		return cfg
	}
	if cfg.OrderByColor {
		fmt.Println("  Color ordering: ON")
	} else {
		fmt.Println("  Color ordering: OFF")
	}
	return cfg
}

func printUsage() {
	fmt.Println("Usage: customls [flags] [path]")
	fmt.Println("       customls <command> [path]")
	fmt.Println()
	fmt.Println("Flags (combine them: -al, -l1):")
	fmt.Println("  -a         Include hidden files")
	fmt.Println("  -l         Long format (perm, size, mtime)")
	fmt.Println("  -1         One entry per line")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  set        Interactively assign colors to items")
	fmt.Println("  colors     Show all custom color assignments")
	fmt.Println("  order      Toggle sorting by color (on/off)")
	fmt.Println("  version    Show version")
	fmt.Println("  help       Show this help message")
	fmt.Println()
	fmt.Println("Output is one per line with no color when piped. Set NO_COLOR")
	fmt.Println("to turn color off everywhere. Colors live in")
	fmt.Println("  " + configPath())
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  customls              List current directory")
	fmt.Println("  customls -la ~        Long listing of home, dotfiles too")
	fmt.Println("  customls order        Toggle color ordering")
	fmt.Println("  customls set          Pick items to colorize")
}

// parseArgs sorts the command line into a command (or "" to list), flags,
// and a path. Flags and the path can come in any order.
func parseArgs(args []string) (cmd string, opts listOpts, path string, err error) {
	for _, a := range args {
		switch a {
		case "help", "--help", "-h":
			return "help", opts, "", nil
		case "version", "--version", "-v":
			return "version", opts, "", nil
		case "set", "colors", "order":
			if cmd != "" {
				return "", opts, "", fmt.Errorf("one command at a time (got %s and %s)", cmd, a)
			}
			cmd = a
			continue
		}

		if strings.HasPrefix(a, "-") && len(a) > 1 {
			for _, ch := range a[1:] {
				switch ch {
				case 'a':
					opts.all = true
				case 'l':
					opts.long = true
				case '1':
					opts.one = true
				default:
					return "", opts, "", fmt.Errorf("unknown flag -%c (try customls help)", ch)
				}
			}
			continue
		}

		if path != "" {
			return "", opts, "", fmt.Errorf("one path at a time (got %s and %s)", path, a)
		}
		path = a
	}
	return cmd, opts, path, nil
}

func main() {
	cmd, opts, path, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "customls: %v\n", err)
		os.Exit(2)
	}
	if path == "" {
		path = "."
	}

	cfg := loadConfig()
	color := useColor()

	switch cmd {
	case "help":
		printUsage()
	case "version":
		fmt.Println("customls " + version)
	case "set":
		cfg = interactiveSet(path, cfg)
		fmt.Println()
		if err := list(os.Stdout, path, cfg, listOpts{}, color, getTermWidth()); err != nil {
			fmt.Fprintf(os.Stderr, "customls: %v\n", err)
			os.Exit(1)
		}
	case "colors":
		showColors(os.Stdout, cfg, color)
	case "order":
		toggleOrder(cfg)
	default:
		if err := list(os.Stdout, path, cfg, opts, color, getTermWidth()); err != nil {
			fmt.Fprintf(os.Stderr, "customls: %v\n", err)
			os.Exit(1)
		}
	}
}
