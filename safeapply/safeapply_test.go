package safeapply

import (
	"os"
	"testing"
)

func TestParseBlocks(t *testing.T) {
	text := "// File: main.go\n```go\npackage main\n```"
	blocks := ParseBlocks(text)
	if len(blocks) != 1 || blocks[0].Path != "main.go" {
		t.Errorf("Expected 1 block for main.go, got %v", blocks)
	}
}

func TestSafeWrite_PathTraversal(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "test")
	defer os.RemoveAll(tmpDir)

	err := SafeWrite(tmpDir, "../../evil.go", "hacked")
	if err == nil {
		t.Errorf("Expected path traversal to be blocked")
	}
}
