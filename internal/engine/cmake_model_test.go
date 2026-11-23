package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/scryer-labs/scryer-cpp-engine/internal/ipc"
)

func TestIncludedCMakeModelSharesFileAndIPCEntrypointOwnership(t *testing.T) {
	root := t.TempDir()
	for name, source := range map[string]string{
		"CMakeLists.txt":  "project(example)\nset(EXE demo)\ninclude(cmake/app.cmake)\n",
		"cmake/app.cmake": "add_executable(${EXE} main.cpp)\ntarget_include_directories(${EXE} PRIVATE include)\n",
		"main.cpp": `#include <api.h>
int main(){CreateNamedPipeA("\\\\.\\pipe\\example",PIPE_ACCESS_DUPLEX,0,1,4096,4096,0,nullptr);return helper();}
`,
		"include/api.h": "inline int helper(){return 0;}\n",
	} {
		file := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	request := Request{}
	request.Scan.Repository = root
	request.Scan.Facts = []string{"architecture", "files", "ipc"}
	report, failure := request.scan(context.Background())
	if failure != nil {
		t.Fatal(failure)
	}
	architecture := report.Facts["architecture"].(architecture)
	if len(architecture.Components) != 1 || architecture.Components[0].Name != "demo" {
		t.Fatal(architecture)
	}
	id := architecture.Components[0].ID
	files := report.Facts["files"].(filesReport)
	foundFile, foundInclude := false, false
	for _, file := range files.Files {
		if file.Path == "main.cpp" && len(file.ComponentIDs) == 1 && file.ComponentIDs[0] == id {
			foundFile = true
		}
	}
	for _, ref := range files.References {
		if ref.File == "main.cpp" && ref.To == "include/api.h" {
			foundInclude = true
		}
	}
	if !foundFile || !foundInclude {
		t.Fatal(files)
	}
	communication := report.Facts["ipc"].(ipc.Facts)
	foundEndpoint := false
	for _, endpoint := range communication.Windows.NamedPipe.Endpoints {
		for _, entry := range endpoint.Entrypoints {
			if entry.File == "main.cpp" && len(entry.ComponentIDs) == 1 && entry.ComponentIDs[0] == id {
				foundEndpoint = true
			}
		}
	}
	if !foundEndpoint {
		t.Fatalf("missing source-context-aware IPC ownership: %+v", communication.Windows.NamedPipe)
	}
}
