package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHeaderProvidersAreEvidenceCandidatesNotDeclaredOwnership(t *testing.T) {
	root := t.TempDir()
	for name, source := range map[string]string{
		"CMakeLists.txt":     "add_library(core impl.c)\nadd_library(mock mock.c)\nadd_executable(app main.c)\ntarget_include_directories(core PUBLIC include)\ntarget_include_directories(mock PRIVATE include)\ntarget_include_directories(app PRIVATE include)\n",
		"include/api.h":      "int service(void);\nint unique(void);\n",
		"include/umbrella.h": "#include <api.h>\n",
		"impl.c":             "#include <api.h>\nint service(void){return 1;}\nint unique(void){return 2;}\n",
		"mock.c":             "int service(void){return 0;}\n",
		"main.c":             "#include <umbrella.h>\nint main(void){return unique();}\n",
	} {
		file := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	request := Request{}
	request.Scan.Repository = root
	request.Scan.Facts = []string{"files"}
	report, failure := request.scan(context.Background())
	if failure != nil {
		t.Fatal(failure)
	}
	files := report.Facts["files"].(filesReport)
	for _, file := range files.Files {
		if file.Path == "include/api.h" {
			if len(file.ComponentIDs) != 0 || file.Ownership.Status != "ambiguous" || len(file.Ownership.CandidateComponentIDs) != 2 {
				t.Fatal(file)
			}
			if len(file.Ownership.Exposures) != 1 || file.Ownership.Exposures[0].Visibility != "PUBLIC" {
				t.Fatal(file.Ownership)
			}
		}
		if file.Path == "impl.c" && file.Ownership.Status != "declared" {
			t.Fatal(file)
		}
		if file.Path == "include/umbrella.h" && len(file.Ownership.CandidateComponentIDs) != 2 {
			t.Fatal(file)
		}
	}
}
