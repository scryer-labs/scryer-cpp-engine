package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/scryer-labs/scryer-cpp-engine/internal/selection"
)

func TestSelectedScanUsesOneScopeForCMakeAndSourceCollectors(t *testing.T) {
	root := t.TempDir()
	for file, source := range map[string]string{"CMakeLists.txt": "add_library(app app.cpp)\n", "app.cpp": "int visible(){return 1;}\n", "hidden.cpp": "int excluded(){return 2;}\n", "hidden.cmake": "add_library(unwanted hidden.cpp)\n"} {
		if err := os.WriteFile(filepath.Join(root, file), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	files := []string{"CMakeLists.txt", "app.cpp"}
	data, _ := json.Marshal(map[string]any{"schemaVersion": 1, "root": root, "files": files})
	manifest := filepath.Join(t.TempDir(), "selection.json")
	if err := os.WriteFile(manifest, data, 0600); err != nil {
		t.Fatal(err)
	}
	request := Request{}
	request.Scan.Repository = root
	request.Scan.Facts = []string{"architecture", "files", "call-graph"}
	request.Scan.FileSelection = &selection.Reference{Path: manifest, SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), FileCount: len(files)}
	report, failure := request.scan(context.Background())
	if failure != nil {
		t.Fatal(failure)
	}
	if report.Selection == nil || report.Selection.FileCount != 2 {
		t.Fatal("missing scope acknowledgment")
	}
	graph := report.Facts["call-graph"].(callGraph)
	if len(graph.Symbols) != 1 || graph.Symbols[0].Name != "visible" {
		t.Fatal(graph.Symbols)
	}
	architecture := report.Facts["architecture"].(architecture)
	if len(architecture.Components) != 1 || architecture.Components[0].Name != "app" {
		t.Fatal(architecture)
	}
}
