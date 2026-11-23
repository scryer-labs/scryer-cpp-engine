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

func TestCatchRegistrationRequiresCompilerConstructorAndHeader(t *testing.T) {
	usr := "c:@N@Catch@S@AutoReg@F@AutoReg#arguments"
	header := "/deps/catch2/internal/catch_test_registry.hpp"
	if result := Registration(usr, header, "cases.cpp", 3, 1); result == nil || result.Evidence[0].Framework != "catch2" {
		t.Fatal(result)
	}
	if Registration("AutoReg", header, "cases.cpp", 3, 1) != nil || Registration(usr, "/business/registry.hpp", "cases.cpp", 3, 1) != nil {
		t.Fatal("accepted spelling without compiler/declaration evidence")
	}
}
