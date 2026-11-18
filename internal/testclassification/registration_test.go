package testclassification

import "testing"

func TestRegistrationRequiresCompilerIdentityAndFrameworkDeclaration(t *testing.T) {
	usr := "c:@N@testing@N@internal@F@MakeAndRegisterTestInfo#arguments"
	header := "/deps/googletest/include/gtest/internal/gtest-internal.h"
	if result := Registration(usr, header, "cases.cpp", 4, 1); result == nil || result.Scope != "test-only" || result.Evidence[0].File != "cases.cpp" {
		t.Fatal("compiler-bound test registration was not retained")
	}
	for _, observation := range [][2]string{{"MakeAndRegisterTestInfo", header}, {usr, "/business/register.h"}, {"c:@N@business@F@MakeAndRegisterTestInfo#arguments", header}} {
		if Registration(observation[0], observation[1], "tests/example_test.cpp", 1, 1) != nil {
			t.Fatal("names or a test-like path must not establish a test role")
		}
	}
}
