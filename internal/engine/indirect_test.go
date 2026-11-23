package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMacroForwardingAndTypedCallbackCandidates(t *testing.T) {
	root := t.TempDir()
	source := `struct callbacks { void (*entry)(void); };
struct context { struct callbacks *ops; };
void concrete(void){}
void alternate(void){}
void implementation(void){}
#define FORWARD() implementation()
#define INSTALL(fn) static struct callbacks instance = { fn };
INSTALL(concrete)
static struct callbacks other = {alternate};
void dispatch(struct context *ctx){ctx->ops->entry();FORWARD();}
`
	if err := os.WriteFile(filepath.Join(root, "callbacks.c"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	facts, err := scanSources(context.Background(), root, false, true, false, false)
	if err != nil {
		t.Fatal(err)
	}
	graph := facts.graph
	foundField, foundMacro := false, false
	for _, call := range graph.Calls {
		if call.Callee == "ctx->ops->entry" {
			if len(call.CandidateIDs) != 2 || call.CandidateBasis != "struct-field-initializer" {
				t.Fatal(call, graph.SymbolReferences)
			}
			foundField = true
		}
		if call.Callee == "FORWARD" {
			if len(call.CandidateIDs) != 1 || call.CandidateBasis != "macro-forwarding" {
				t.Fatal(call, graph.MacroExpansions)
			}
			foundMacro = true
		}
	}
	if !foundField || !foundMacro {
		t.Fatal(graph)
	}
}
