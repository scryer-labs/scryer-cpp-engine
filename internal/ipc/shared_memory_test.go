package ipc

import (
	"context"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
	cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
)

func TestSharedMemoryTracesNamedMappingThroughFactory(t *testing.T) {
	files := []struct{ path, code string }{
		{"resource.h", `
namespace domain {
const auto Name = L"Local\\resource";
class Resource {
public:
	void start() { allocate(); }
	void attach() {
		auto handle = OpenFileMappingW(FILE_MAP_ALL_ACCESS, false, Name);
		MapViewOfFileEx(handle, FILE_MAP_COPY, 0, 0, 0, nullptr);
	}
private:
	void allocate() {
		CreateFileMappingW(INVALID_HANDLE_VALUE, nullptr, PAGE_READWRITE, 0, 512, Name);
		CreateFileMappingW(INVALID_HANDLE_VALUE, nullptr, mode(), 0, 512, name());
	}
};
template<class T> struct Factory {
	static void start() { resource()->start(); }
	static void attach() { resource()->attach(); }
	static std::shared_ptr<Resource> resource() { return makeResource(); }
};
}
`},
		{"app.cpp", `
#ifdef FEATURE
using Startup = domain::Factory<int>;
auto initial = Startup::start();
#endif
`},
		{"test/client.cpp", `using Startup = domain::Factory<int>; auto initial = Startup::attach();`},
	}
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(sitter.NewLanguage(cpp.Language())); err != nil {
		t.Fatal(err)
	}
	var collector Collector
	for _, file := range files {
		source := []byte(file.code)
		tree := parser.Parse(source, nil)
		err := collector.Observe(context.Background(), tree.RootNode(), file.path, source)
		tree.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	facts := collector.Facts().Windows.SharedMemory
	if facts == nil || len(facts.Mappings) != 1 || len(facts.Views) != 1 || len(facts.Unresolved) != 1 {
		t.Fatalf("unexpected shared memory facts: %+v", facts)
	}
	mapping := facts.Mappings[0]
	if mapping.Name != `Local\resource` || len(mapping.Operations) != 2 {
		t.Fatalf("unexpected mapping: %+v", mapping)
	}
	for _, operation := range mapping.Operations {
		want := "app.cpp"
		if operation.Operation == "open" {
			want = "test/client.cpp"
		}
		if operation.AccessMode != "read-write" || operation.NameLocation == nil ||
			len(operation.Entrypoints) != 1 || operation.Entrypoints[0].File != want {
			t.Fatalf("unexpected operation: %+v", operation)
		}
		if operation.Operation == "create" {
			conditions := operation.Entrypoints[0].Conditions
			if len(conditions) != 1 || conditions[0] != "#ifdef FEATURE" {
				t.Fatalf("expected the creator's compile-time guard, got %v", conditions)
			}
		}
	}
	view := facts.Views[0]
	if view.AccessMode != "copy-on-write" || view.HandleSourceCandidateID != mapping.Operations[0].ID {
		t.Fatalf("unexpected map view: %+v", view)
	}
}
