package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/SiliconSailor/safeapply/safeapply"
)

type Config struct {
	ProjectRoot string `json:"project_root"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		wd, _ := os.Getwd()
		cfg := &Config{ProjectRoot: wd}
		out, _ := json.MarshalIndent(cfg, "", "  ")
		if werr := os.WriteFile(path, out, 0644); werr != nil {
			return nil, fmt.Errorf("create default config: %w", werr)
		}
		fmt.Printf("[*] Created default config: %s\n", path)
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

func readInput(reader *bufio.Reader) (string, error) {
	var lines []string
	emptyCount := 0

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		isEOF := err == io.EOF
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			emptyCount++
			if emptyCount >= 2 {
				if len(lines) > 0 {
					lines = lines[:len(lines)-1]
				}
				return strings.Join(lines, "\n"), nil
			}
			lines = append(lines, line)
			continue
		}

		emptyCount = 0
		lines = append(lines, line)

		if isEOF {
			return strings.Join(lines, "\n"), nil
		}
	}
}

func askMenu(reader *bufio.Reader) byte {
	for {
		fmt.Print("Actions: [A]ccept and write / [S]kip specific / [Q]uit to main loop: ")
		line, _ := reader.ReadString('\n')
		line = strings.ToLower(strings.TrimSpace(line))
		if len(line) > 0 {
			switch line[0] {
			case 'a': return 'A'
			case 's': return 'S'
			case 'q': return 'Q'
			}
		}
		fmt.Println("[ERROR] invalid choice")
	}
}

func parseIndices(s string, max int) (map[int]bool, error) {
	result := make(map[int]bool)
	s = strings.TrimSpace(s)
	if s == "" { return result, nil }
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" { continue }
		idx, err := strconv.Atoi(part)
		if err != nil { return nil, fmt.Errorf("invalid index: %q", part) }
		if idx < 1 || idx > max { return nil, fmt.Errorf("out of range: %d (valid: 1-%d)", idx, max) }
		result[idx] = true
	}
	return result, nil
}

func copyToClipboard(text string) error {
	cmd := exec.Command("cmd", "/c", "clip")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func main() {
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	configPath := filepath.Join(exeDir, "bridge_config.json")

	cfg, err := loadConfig(configPath)
	if err != nil {
		fmt.Printf("[FATAL] config: %v\n", err)
		return
	}

	if _, err := os.Stat(cfg.ProjectRoot); os.IsNotExist(err) {
		fmt.Printf("[FATAL] project_root not found: %s\n", cfg.ProjectRoot)
		return
	}

	fmt.Println("╔════════════════════════════════════════════╗")
	fmt.Println("║  KiraBufferBridge v1.0                     ║")
	fmt.Println("╚════════════════════════════════════════════╝")
	fmt.Printf("    Project root: %s\n", cfg.ProjectRoot)

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println("\n=== PASTE LLM RESPONSE (Press Enter twice consecutively to submit, type '!quit' to exit) ===")
		
		input, err := readInput(reader)
		if err != nil { continue }
		input = strings.TrimSpace(input)
		
		// FIX: Защита от пустого ввода
		if input == "" {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if input == "!quit" || input == "!exit" {
			break
		}

		blocks := safeapply.ParseBlocks(input)
		files := safeapply.Group(blocks)

		if len(files) == 0 {
			fmt.Println("[WARN] No code blocks with path directives found in text.")
			continue
		}

		skipSet := make(map[int]bool)
		var writtenCount int
		var reportLines []string

	menuLoop:
		for {
			fmt.Println("\n━━ Detected Files ━━")
			for _, f := range files {
				lines := strings.Count(f.Content, "\n") + 1
				mark := ""
				if f.Incomplete { mark = " ⚠ [INCOMPLETE]" }
				skipMark := ""
				if skipSet[f.Index] { skipMark = " [SKIP]" }
				fmt.Printf("  %d. %s (%d lines)%s%s\n", f.Index, f.OriginalPath, lines, mark, skipMark)
			}

			choice := askMenu(reader)
			switch choice {
			case 'A':
				writtenCount = 0
				reportLines = nil
				for _, f := range files {
					if skipSet[f.Index] { continue }
					
					lines := strings.Count(f.Content, "\n") + 1
					err := safeapply.SafeWrite(cfg.ProjectRoot, f.OriginalPath, f.Content)
					if err != nil {
						fmt.Printf("  [REJECTED] %s: %v\n", f.OriginalPath, err)
						continue
					}
					
					fmt.Printf("  [WROTE] %s (%d lines)\n", f.OriginalPath, lines)
					reportLines = append(reportLines, fmt.Sprintf("[SUCCESS] Written: %s (%d lines)", f.OriginalPath, lines))
					writtenCount++
				}
				break menuLoop
			case 'S':
				fmt.Print("Skip file numbers (comma-sep, Enter=none): ")
				line, _ := reader.ReadString('\n')
				ss, err := parseIndices(line, len(files))
				if err == nil { skipSet = ss }
			case 'Q':
				break menuLoop
			}
		}

		if writtenCount > 0 && len(reportLines) > 0 {
			reportText := strings.Join(reportLines, "\n")
			fmt.Print("\nDo you want to copy the execution report to clipboard? [Y/n]: ")
			ans, _ := reader.ReadString('\n')
			ans = strings.ToLower(strings.TrimSpace(ans))
			if ans == "" || ans == "y" {
				if err := copyToClipboard(reportText); err == nil {
					fmt.Println("[SUCCESS] Report copied to clipboard.")
				}
			}
		}
	}
}