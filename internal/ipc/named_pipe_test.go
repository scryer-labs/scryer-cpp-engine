package ipc

import (
	"context"
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
	cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
)

func TestNamedPipeFactsFollowArgumentsWithoutDomainNames(t *testing.T) {
	source := []byte(`
namespace app {
std::wstring convert(std::string_view data) {
	return std::filesystem::path(data).c_str();
}
struct Resource {
	Resource(std::string_view value);
	void run();
	std::string field;
};
Resource::Resource(std::string_view value) : field{value} {}
void Resource::run() { CreateNamedPipeW(convert(field).c_str(), 0); }
auto X(std::string_view Y) { return std::make_unique<Resource>(Y); }
const auto Literal = R"(\\.\pipe\one)";
void start() { CreateNamedPipeW(Literal, 0); }
void other() { CreateFileW(L"ordinary.txt", 0); }
}
`)
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(sitter.NewLanguage(cpp.Language())); err != nil {
		t.Fatal(err)
	}
	tree := parser.Parse(source, nil)
	defer tree.Close()
	var collector Collector
	if err := collector.Observe(context.Background(), tree.RootNode(), "tests/source.cpp", source); err != nil {
		t.Fatal(err)
	}
	facts := collector.Facts().Windows.NamedPipe
	if len(facts.Endpoints) != 1 || facts.Endpoints[0].Name != `\\.\pipe\one` || facts.Endpoints[0].SourceRole != "test" {
		t.Fatalf("unexpected concrete endpoints: %+v", facts.Endpoints)
	}
	if len(facts.Parameters) != 2 {
		t.Fatalf("expected the constructor and its factory: %+v", facts.Parameters)
	}
	factory := facts.Parameters[1]
	if factory.Symbol != "app::X" || factory.ParameterIndex != 0 || factory.Role != "server" {
		t.Fatalf("unexpected factory: %+v", factory)
	}
	var operations []string
	for _, evidence := range factory.Evidence {
		operations = append(operations, evidence.Operation)
	}
	if strings.Join(operations, ",") != "factory-construction,member-initializer,name-forwarding-call,name-forwarding-definition,native-api" {
		t.Fatalf("unexpected parameter evidence: %+v", factory.Evidence)
	}
}
