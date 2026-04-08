package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

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

func loadConfig() Config {
	cfg := Config{Colors: make(map[string]string)}
	data, err := os.ReadFile(configPath())
	if err != nil {
		return cfg
	}
	json.Unmarshal(data, &cfg)
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

func colorize(name string, isDir bool, cfg Config) string {
	if customColor, ok := cfg.Colors[name]; ok {
		if code, ok2 := colorCodes[customColor]; ok2 {
			return code + name + resetCode
		}
	} else if isDir {
		return defaultDirColor + name + resetCode
	}
	return name
}

func listDir(dir string, cfg Config) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "customls: %v\n", err)
		os.Exit(1)
	}

	type item struct {
		name  string
		isDir bool
	}

	var items []item
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			items = append(items, item{name: e.Name(), isDir: e.IsDir()})
		}
	}
	if cfg.OrderByColor {
		sort.SliceStable(items, func(i, j int) bool {
			ri, rj := colorRank(items[i].name, items[i].isDir, cfg), colorRank(items[j].name, items[j].isDir, cfg)
			if ri != rj {
				return ri < rj
			}
			return strings.ToLower(items[i].name) < strings.ToLower(items[j].name)
		})
	} else {
		sort.Slice(items, func(i, j int) bool {
			return strings.ToLower(items[i].name) < strings.ToLower(items[j].name)
		})
	}

	if len(items) == 0 {
		return
	}

	maxLen := 0
	for _, it := range items {
		if len(it.name) > maxLen {
			maxLen = len(it.name)
		}
	}

	termWidth := getTermWidth()
	colWidth := maxLen + 2
	if colWidth < 4 {
		colWidth = 4
	}
	numCols := termWidth / colWidth
	if numCols < 1 {
		numCols = 1
	}

	// Column-major layout: fill top-to-bottom, then next column
	numRows := (len(items) + numCols - 1) / numCols

	for row := 0; row < numRows; row++ {
		for col := 0; col < numCols; col++ {
			idx := col*numRows + row
			if idx >= len(items) {
				break
			}
			it := items[idx]
			display := colorize(it.name, it.isDir, cfg)

			isLast := col == numCols-1 || (col+1)*numRows+row >= len(items)
			if isLast {
				fmt.Println(display)
			} else {
				padding := colWidth - len(it.name)
				if padding < 1 {
					padding = 1
				}
				fmt.Print(display + strings.Repeat(" ", padding))
			}
		}
	}
}

func listDirLong(dir string, cfg Config, showAll bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "customls: %v\n", err)
		os.Exit(1)
	}

	type fileItem struct {
		name    string
		isDir   bool
		info    os.FileInfo
	}

	var items []fileItem
	for _, e := range entries {
		if !showAll && strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, fileItem{name: e.Name(), isDir: e.IsDir(), info: info})
	}
	if cfg.OrderByColor {
		sort.SliceStable(items, func(i, j int) bool {
			ri, rj := colorRank(items[i].name, items[i].isDir, cfg), colorRank(items[j].name, items[j].isDir, cfg)
			if ri != rj {
				return ri < rj
			}
			return strings.ToLower(items[i].name) < strings.ToLower(items[j].name)
		})
	} else {
		sort.Slice(items, func(i, j int) bool {
			return strings.ToLower(items[i].name) < strings.ToLower(items[j].name)
		})
	}

	if len(items) == 0 {
		return
	}

	// Find max size width for alignment
	maxSize := 0
	for _, it := range items {
		s := len(formatSize(it.info.Size()))
		if s > maxSize {
			maxSize = s
		}
	}

	for _, it := range items {
		perm := it.info.Mode().String()
		size := formatSize(it.info.Size())
		mod := it.info.ModTime().Format("Jan _2 15:04")
		display := colorize(it.name, it.isDir, cfg)
		fmt.Printf("%s  %*s  %s  %s\n", perm, maxSize, size, mod, display)
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

func interactiveSet(dir string, cfg Config) Config {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "customls: %v\n", err)
		os.Exit(1)
	}

	type item struct {
		name  string
		isDir bool
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
		fmt.Println("\n  Select an item to colorize:\n")
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

func showColors(cfg Config) {
	if len(cfg.Colors) == 0 {
		fmt.Println("  No custom colors set. Run 'customls set' to add some.")
		return
	}
	fmt.Println("\n  Custom colors:")
	for name, color := range cfg.Colors {
		code := colorCodes[color]
		fmt.Printf("  %s%-20s%s  %s\n", code, name, resetCode, color)
	}
	fmt.Println()
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
	fmt.Println("Usage: customls [command|flags] [path]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  (none)     List directory with custom colors")
	fmt.Println("  -l         Long format with custom colors")
	fmt.Println("  -al        Long format including hidden files")
	fmt.Println("  set        Interactively assign colors to items")
	fmt.Println("  colors     Show all custom color assignments")
	fmt.Println("  order      Toggle sorting by color (on/off)")
	fmt.Println("  help       Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  customls              List current directory")
	fmt.Println("  customls -l           Long listing with colors")
	fmt.Println("  customls order        Toggle color ordering")
	fmt.Println("  customls set          Pick items to colorize")
}

func main() {
	cfg := loadConfig()

	if len(os.Args) < 2 {
		listDir(".", cfg)
		return
	}

	switch os.Args[1] {
	case "-l":
		dir := "."
		if len(os.Args) > 2 {
			dir = os.Args[2]
		}
		listDirLong(dir, cfg, false)
	case "-al", "-la":
		dir := "."
		if len(os.Args) > 2 {
			dir = os.Args[2]
		}
		listDirLong(dir, cfg, true)
	case "set":
		dir := "."
		if len(os.Args) > 2 {
			dir = os.Args[2]
		}
		cfg = interactiveSet(dir, cfg)
		fmt.Println()
		listDir(dir, cfg)
	case "colors":
		showColors(cfg)
	case "order":
		toggleOrder(cfg)
	case "help", "--help", "-h":
		printUsage()
	default:
		// Treat as a path
		listDir(os.Args[1], cfg)
	}
}
