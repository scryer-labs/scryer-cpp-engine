package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func Test_should_preserve_cpp_correspondence_when_comments_and_offsets_change(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "sample.cpp")
	before := "namespace demo { struct Item { int value(int n) { return n+1; } int value() { return 2; } }; }"
	collect := func(source string) impactSnapshot {
		t.Helper()
		if err := os.WriteFile(file, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		result, err := collectImpactSnapshot(context.Background(), root, false)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	left := collect(before)
	right := collect("// shifted location\nnamespace demo { struct Item { int value(int n) { return n /* comment */ + 1; } int value() { return 2; } }; }")
	if len(left.Symbols) != 2 || len(right.Symbols) != 2 || left.Symbols[0].Key == left.Symbols[1].Key {
		t.Fatal("overloads must have separate correspondence keys")
	}
	for i, old := range left.Symbols {
		current := right.Symbols[i]
		if old.Key != current.Key || old.Fingerprint != current.Fingerprint || old.ID == current.ID {
			t.Fatal("comments or location offsets altered semantic comparison inputs")
		}
	}
	changed := collect("namespace demo { struct Item { int value(int n) { return n+3; } int value() { return 2; } }; }")
	if changed.Symbols[0].Key != left.Symbols[0].Key || changed.Symbols[0].Fingerprint == left.Symbols[0].Fingerprint {
		t.Fatal("body modification must change fingerprint while retaining correspondence")
	}
}
