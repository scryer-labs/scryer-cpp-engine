package cmake

import (
	"context"
	"testing"
)

func TestRuntimeEnvironmentRemainsDeclarationEvidence(t *testing.T) {
	facts := Facts{}
	if err := facts.scanFile(context.Background(), "CMakeLists.txt", `set(ENV{LD_PRELOAD} "${CMAKE_BINARY_DIR}/liboverride.so")`); err != nil {
		t.Fatal(err)
	}
	if len(facts.RuntimeBindings) != 1 || facts.RuntimeBindings[0].Status != "declared-environment" {
		t.Fatal(facts.RuntimeBindings)
	}
}
