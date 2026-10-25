package safeapply

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type CodeBlock struct {
	Path       string
	Content    string
	Incomplete bool
}

type GroupedFile struct {
	Index        int
	OriginalPath string
	Content      string
	Incomplete   bool
}

var pathDirectiveRe = regexp.MustCompile(`(?i)^\s*(?://|\*\*)\s*file\s*:?\s*\**\s*([^\s*]+)\s*\**\s*$`)

func ParseBlocks(text string) []CodeBlock {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	var blocks []CodeBlock
	var currentPath string
	inBlock := false
	var blockLines []string

	flush := func(unclosed bool) {
		if !inBlock {
			return
		}
		content := strings.Join(blockLines, "\n")
		path := currentPath
		if path == "" && len(blockLines) > 0 {
			if m := pathDirectiveRe.FindStringSubmatch(blockLines[0]); m != nil {
				path = m[1]
				content = strings.Join(blockLines[1:], "\n")
			}
		}
		blocks = append(blocks, CodeBlock{Path: path, Content: content, Incomplete: unclosed})
		currentPath = ""
		blockLines = nil
		inBlock = false
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inBlock {
			if m := pathDirectiveRe.FindStringSubmatch(line); m != nil {
				currentPath = m[1]
				continue
			}
			if strings.HasPrefix(trimmed, "```") {
				inBlock = true
				blockLines = nil
				continue
			}
			continue
		}
		if trimmed == "```" {
			flush(false)
			continue
		}
		blockLines = append(blockLines, line)
	}
	if inBlock {
		flush(true)
	}
	return blocks
}

func Group(blocks []CodeBlock) []GroupedFile {
	order := []string{}
	grouped := make(map[string]*GroupedFile)

	for _, b := range blocks {
		if b.Path == "" {
			continue
		}
		key := strings.ToLower(filepath.ToSlash(b.Path))
		if g, exists := grouped[key]; exists {
			g.Content += "\n\n" + b.Content
			if b.Incomplete {
				g.Incomplete = true
			}
		} else {
			grouped[key] = &GroupedFile{
				OriginalPath: b.Path,
				Content:      b.Content,
				Incomplete:   b.Incomplete,
			}
			order = append(order, key)
		}
	}

	result := make([]GroupedFile, 0, len(order))
	for i, key := range order {
		g := grouped[key]
		g.Index = i + 1
		result = append(result, *g)
	}
	return result
}

func SafeWrite(projectRoot, relPath, content string) error {
	projectRootAbs, err := filepath.Abs(projectRoot)
	if err != nil {
		return fmt.Errorf("abs project_root: %w", err)
	}
	derivedAbs := filepath.Clean(filepath.Join(projectRootAbs, relPath))
	rel, err := filepath.Rel(projectRootAbs, derivedAbs)
	if err != nil {
		return fmt.Errorf("rel error: %w", err)
	}

	isEscaping := rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
	if isEscaping || filepath.IsAbs(rel) {
		return fmt.Errorf("path escapes project root")
	}

	checkPath := rel
	if len(checkPath) >= 2 && checkPath[1] == ':' {
		checkPath = checkPath[2:]
	}
	if strings.Contains(checkPath, ":") {
		return fmt.Errorf("invalid path character (ADS protection)")
	}

	if _, statErr := os.Stat(derivedAbs); statErr == nil {
		if err := BackupFile(derivedAbs); err != nil {
			return fmt.Errorf("backup failed: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(derivedAbs), 0755); err != nil {
		return fmt.Errorf("mkdir error: %w", err)
	}
	if err := os.WriteFile(derivedAbs, []byte(content), 0644); err != nil {
		return fmt.Errorf("write error: %w", err)
	}
	return nil
}

func BackupFile(src string) error {
	now := time.Now()
	ts := now.Format("2006-01-02-150405") + fmt.Sprintf("-%03d", now.Nanosecond()/1e6)
	backupPath := src + "." + ts + ".bak"

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(backupPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		os.Remove(backupPath)
		return err
	}
	return nil
}
