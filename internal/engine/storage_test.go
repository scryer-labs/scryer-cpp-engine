package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func staticFixture(t *testing.T, files map[string]string, includes bool) sourceFacts {
	t.Helper()
	root := filepath.Join(t.TempDir(), "source space Ω")
	for name, source := range files {
		file := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	facts, err := scanSources(context.Background(), root, includes, true, false, false)
	if err != nil {
		t.Fatal(err)
	}
	return facts
}

func Test_should_prefer_file_internal_C_helpers_without_changing_locations(t *testing.T) {
	for _, storage := range []string{"static inline", "inline static", "inline /* storage */\r\nstatic"} {
		t.Run(strings.ReplaceAll(storage, "\r\n", "-"), func(t *testing.T) {
			source := storage + " int helper(void) { return 1; }\r\nint wrapper(void) { return helper(); }\r\n"
			graph := staticFixture(t, map[string]string{"a.c": source, "b.c": source}, false).graph
			symbols := map[string]functionSymbol{}
			for _, s := range graph.Symbols {
				symbols[s.ID] = s
			}
			if len(graph.Calls) != 2 {
				t.Fatal(graph.Calls)
			}
			for _, c := range graph.Calls {
				target := symbols[c.CandidateID]
				if c.Resolution != "name-candidate" || c.CandidateBasis != "file-internal-definition" || target.Name != "helper" || target.Definition.File != c.Site.File {
					t.Fatal(c, target)
				}
				if !strings.Contains(c.ID, ":") || c.Site.Start.Line != uint(strings.Count(storage, "\n")+2) || c.Site.Start.Column != 28 {
					t.Fatal(c.Site)
				}
			}
		})
	}
}

func Test_should_keep_static_shadowing_and_conditional_boundaries(t *testing.T) {
	cases := map[string]string{
		"malformed":        "static int helper(void){return @;}\nint wrapper(void){return helper();}",
		"typedef-callback": "static int helper(void){return 1;}\nint wrapper(void){typedef int (*Callback)(void); Callback helper = 0; return helper();}",
		"typedef":          "static int helper(void){return 1;}\nint wrapper(void){typedef int helper; return helper();}",
		"parameter":        "static int helper(void){return 1;}\nint wrapper(int (*helper)(void)){return helper();}",
		"local":            "static int helper(void){return 1;}\nint wrapper(void){int (*unused)(void), (*helper)(void); return helper();}",
		"macro":            "static int helper(void){return 1;}\n#define helper() other()\nint wrapper(void){return helper();}",
		"empty-macro":      "static int helper(void){return 1;}\n#define helper\nint wrapper(void){return helper();}",
		"conditional":      "#ifdef OPTION\nstatic int helper(void){return 1;}\n#endif\nint wrapper(void){return helper();}",
		"duplicates":       "#ifdef OPTION\nstatic int helper(void){return 1;}\n#else\nstatic int helper(void){return 2;}\n#endif\nint wrapper(void){return helper();}",
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			graph := staticFixture(t, map[string]string{"a.c": source}, false).graph
			found := false
			for _, c := range graph.Calls {
				if c.Callee == "helper" {
					found = true
					if c.CandidateID != "" || c.CandidateBasis == "file-internal-definition" {
						t.Fatal(c)
					}
				}
			}
			if !found {
				t.Fatal("missing call", graph)
			}
		})
	}
}

func Test_should_preserve_external_header_and_Cpp_candidate_behavior(t *testing.T) {
	facts := staticFixture(t, map[string]string{
		"api.h":   "static inline int header_helper(void){return 1;}\n",
		"a.c":     "#include \"api.h\"\nint external(void); int a(void){return external()+header_helper();}\n",
		"b.c":     "int external(void){return 1;}\n",
		"one.cpp": "static int cpp_helper(){return 1;}\nstruct One { static int member(){return 1;} }; int one(){return cpp_helper()+One::member();}\n",
		"two.cpp": "static int cpp_helper(){return 2;}\nstruct Two { static int member(){return 2;} }; int two(){return cpp_helper();}\n",
	}, false)
	for _, s := range facts.graph.Symbols {
		if s.Name == "member" && s.fileStatic {
			t.Fatal("class static treated as file static", s)
		}
	}
	checked := 0
	for _, c := range facts.graph.Calls {
		if c.Callee == "external" || c.Callee == "header_helper" {
			checked++
			if c.CandidateID == "" || c.Resolution != "name-candidate" || c.CandidateBasis != "" {
				t.Fatal(c)
			}
		}
		if c.Callee == "cpp_helper" && (c.Resolution != "ambiguous-name" || c.CandidateBasis != "") {
			t.Fatal(c)
		}
	}
	if checked != 2 {
		t.Fatal(facts.graph.Calls)
	}
}

func Test_should_keep_local_binding_boundary_when_static_helper_has_a_prototype(t *testing.T) {
	source := "static int helper(void);\n" +
		"static int helper(void){return 1;}\n" +
		"int wrapper(int (*helper)(void)){return helper();}\n" +
		"int direct(void){return helper();}\n"
	graph := staticFixture(t, map[string]string{"a.c": source}, false).graph
	if len(graph.Calls) != 2 {
		t.Fatal(graph.Calls)
	}
	shadowed, direct := graph.Calls[0], graph.Calls[1]
	if shadowed.Site.Start.Line != 3 || shadowed.Resolution != "local-binding-boundary" || shadowed.CandidateID != "" {
		t.Fatal(shadowed)
	}
	if direct.Site.Start.Line != 4 || direct.Resolution != "name-candidate" || direct.CandidateID == "" || direct.CandidateBasis != "" {
		t.Fatal(direct)
	}
}

func Test_should_exclude_all_literal_static_free_function_spellings_from_provider_evidence(t *testing.T) {
	facts := staticFixture(t, map[string]string{
		"api.h":       "int local_one(void); int local_two(void); int external(void);\n",
		"impl.c":      "inline static int local_one(void){return 1;}\ninline /* gap */\nstatic int local_two(void){return 1;}\nint external(void){return 1;}\n",
		"members.cpp": "struct Item { static int method(){return 1;} };\n",
	}, true)
	for _, d := range facts.api {
		if d.Definition && (d.Name == "local_one" || d.Name == "local_two") {
			t.Fatal("static helper supplied provider evidence", d)
		}
	}
	found := false
	for _, d := range facts.api {
		if d.Definition && d.Name == "external" {
			found = true
		}
	}
	if !found {
		t.Fatal(facts.api)
	}
}

func Test_should_not_treat_C_filename_as_a_compiler_language_guarantee(t *testing.T) {
	for name, source := range map[string]string{
		"struct-method": "static int helper(){return 1;} struct Item { int value(){return 2;} }; int wrapper(){return helper();}",
		"namespace":     "namespace detail { static int helper(){return 1;} } int wrapper(){return helper();}",
		"overload":      "inline static int helper(){return 1;} int helper(int); int wrapper(){return helper(1);}",
		"member":        "static int helper(){return 1;} struct Item { static int helper(){return 2;} int run(){return helper();} };",
	} {
		t.Run(name, func(t *testing.T) {
			graph := staticFixture(t, map[string]string{"mixed.c": source, "other.c": "static int helper(){return 3;}"}, false).graph
			for _, call := range graph.Calls {
				if call.CandidateBasis == "file-internal-definition" {
					t.Fatal(call)
				}
			}
		})
	}
}

func Test_should_preserve_uppercase_C_as_a_Cpp_filename_boundary(t *testing.T) {
	graph := staticFixture(t, map[string]string{"one.C": "static int helper(){return 1;} int one(){return helper();}", "two.C": "static int helper(){return 2;} int two(){return helper();}"}, false).graph
	for _, call := range graph.Calls {
		if call.CandidateID != "" || call.CandidateBasis == "file-internal-definition" {
			t.Fatal(call)
		}
	}
}

func Test_should_not_assign_a_header_to_an_inline_static_implementation(t *testing.T) {
	root := t.TempDir()
	for file, source := range map[string]string{
		"CMakeLists.txt": "add_library(implementation impl.c)\n",
		"api.h":          "int helper(void);\n",
		"impl.c":         "inline /* order */\nstatic int helper(void){return 1;}\n",
	} {
		if err := os.WriteFile(filepath.Join(root, file), []byte(source), 0600); err != nil {
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
		if file.Path == "api.h" && (file.Ownership.Status != "unassigned" || len(file.Ownership.CandidateComponentIDs) != 0) {
			t.Fatal(file)
		}
	}
}

func Test_should_not_treat_nested_prototype_parameters_as_caller_bindings(t *testing.T) {
	graph := staticFixture(t, map[string]string{
		"a.c": "static int helper(void){return 1;} int wrapper(int (*callback)(int helper)){return helper();}",
		"b.c": "static int helper(void){return 2;}",
	}, false).graph
	if len(graph.Calls) != 1 || graph.Calls[0].CandidateBasis != "file-internal-definition" {
		t.Fatal(graph.Calls)
	}
}
