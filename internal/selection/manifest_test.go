package selection

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func selectionFixture(t *testing.T, root string, files []string) *Reference {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"schemaVersion": 1, "root": root, "files": files})
	file := filepath.Join(t.TempDir(), "files.json")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	return &Reference{Path: file, SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), FileCount: len(files)}
}
func TestSelectionRejectsTamperingTraversalAndDuplicatePaths(t *testing.T) {
	root := t.TempDir()
	for _, files := range [][]string{{"../outside.cpp"}, {"x.cpp", "x.cpp"}, {".git/x.cpp"}} {
		if set, err := Load(root, selectionFixture(t, root, files)); err == nil {
			set.Close()
			t.Fatal(files)
		}
	}
	ref := selectionFixture(t, root, []string{})
	ref.SHA256 = "bad"
	if _, err := Load(root, ref); err == nil {
		t.Fatal("accepted altered digest")
	}
	ref = selectionFixture(t, root, []string{})
	ref.FileCount = 2
	if _, err := Load(root, ref); err == nil {
		t.Fatal("accepted wrong count")
	}
}
func TestEmptySelectionIsNotLegacyDirectoryTraversal(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "unselected.cpp"), []byte("int unwanted(){}"), 0600); err != nil {
		t.Fatal(err)
	}
	set, err := Load(root, selectionFixture(t, root, []string{}))
	if err != nil {
		t.Fatal(err)
	}
	defer set.Close()
	if err = set.Walk(context.Background(), func(_ string, _ os.DirEntry, _ error) error { t.Fatal("walked excluded file"); return nil }); err != nil {
		t.Fatal(err)
	}
}
