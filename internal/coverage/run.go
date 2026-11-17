package coverage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/scryer-labs/scryer-cpp-engine/internal/process"
)

type Run struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Profiles int    `json:"profiles"`
	Message  string `json:"message,omitempty"`
}

type Collection struct {
	Status   string    `json:"status"`
	Message  string    `json:"message,omitempty"`
	Runs     []Run     `json:"runs,omitempty"`
	Evidence *Evidence `json:"coverage,omitempty"`
}

func Collect(ctx context.Context, root string, testBinaries, testTargets []string) (Collection, error) {
	if runtime.GOOS != "darwin" {
		return Collection{}, fmt.Errorf("automatic C++ coverage currently requires macOS; use --coverage-file to import existing evidence")
	}
	if _, err := os.Stat(filepath.Join(root, "CMakeLists.txt")); err != nil {
		return Collection{}, fmt.Errorf("automatic C++ coverage requires a CMake project root: %w", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "CMakeCache.txt")); !errors.Is(err, os.ErrNotExist) {
		return Collection{}, fmt.Errorf("automatic coverage needs a source directory without an in-source CMake cache")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	tools, err := appleTools(ctx)
	if err != nil {
		return Collection{}, err
	}
	seen := map[string]bool{}
	for _, relative := range testBinaries {
		clean := filepath.Clean(relative)
		if !filepath.IsLocal(relative) || clean == "." || strings.ContainsRune(relative, 0) || seen[clean] {
			return Collection{}, fmt.Errorf("invalid or duplicate test binary path %q", relative)
		}
		seen[clean] = true
	}
	work, err := os.MkdirTemp("", "scryer-cpp-coverage-")
	if err != nil {
		return Collection{}, err
	}
	defer os.RemoveAll(work)
	absoluteWork, err := filepath.EvalSymlinks(work)
	if err != nil {
		return Collection{}, err
	}
	build := filepath.Join(absoluteWork, "build")
	if err := requestCodeModel(build); err != nil {
		return Collection{}, err
	}
	const instrumentation = "-fprofile-instr-generate -fcoverage-mapping"
	configure := []string{"-S", root, "-B", build, "-G", "Unix Makefiles", "-DBUILD_TESTING=ON", "-DCMAKE_BUILD_TYPE=Debug",
		"-DCMAKE_C_COMPILER=" + tools["clang"], "-DCMAKE_CXX_COMPILER=" + tools["clang++"],
		"-DCMAKE_C_FLAGS=" + strings.TrimSpace(os.Getenv("CFLAGS")+" "+instrumentation),
		"-DCMAKE_CXX_FLAGS=" + strings.TrimSpace(os.Getenv("CXXFLAGS")+" "+instrumentation),
	}
	for _, kind := range []string{"EXE", "SHARED", "MODULE"} {
		configure = append(configure, "-DCMAKE_"+kind+"_LINKER_FLAGS="+strings.TrimSpace(os.Getenv("LDFLAGS")+" -fprofile-instr-generate"))
	}
	if err := command(ctx, build, "", nil, "cmake", configure...); err != nil {
		return Collection{}, fmt.Errorf("configure coverage build: %w", err)
	}
	targets, err := readCodeModel(ctx, build)
	if err != nil {
		return Collection{}, fmt.Errorf("read coverage build targets: %w", err)
	}
	buildArgs := []string{"--build", build, "--parallel"}
	testNames := testBinaries
	if len(testTargets) != 0 {
		testBinaries, err = selectedTargetPaths(targets, testTargets)
		if err != nil {
			return Collection{}, err
		}
		testNames = testTargets
		buildArgs = append(append(buildArgs, "--target"), testTargets...)
	}
	if err := command(ctx, build, "", nil, "cmake", buildArgs...); err != nil {
		return Collection{}, fmt.Errorf("build with coverage instrumentation: %w", err)
	}
	var binaries []string
	for _, relative := range testBinaries {
		path, err := buildArtifact(build, relative)
		if err != nil {
			return Collection{}, fmt.Errorf("find test binary %q relative to the temporary coverage build directory: %w; use --test-target to select a CMake executable by name", relative, err)
		}
		for _, previous := range binaries {
			if previous == path {
				return Collection{}, fmt.Errorf("test binary %q resolves to an already selected executable", relative)
			}
		}
		binaries = append(binaries, path)
	}
	runs, profiles, err := runTests(ctx, absoluteWork, build, testNames, binaries)
	if err != nil {
		return Collection{}, err
	}
	if len(profiles) == 0 {
		return Collection{Status: "unavailable", Message: "No test execution produced coverage profiles.", Runs: runs}, nil
	}
	objects, err := coverageObjects(build, targets, binaries)
	if err != nil {
		return Collection{}, fmt.Errorf("find coverage objects: %w", err)
	}
	evidence, err := exportLLVM(ctx, absoluteWork, root, tools, profiles, objects)
	if err != nil {
		return Collection{}, err
	}
	result := Collection{Status: "collected", Runs: runs, Evidence: &evidence}
	for _, run := range runs {
		if run.Status != "succeeded" {
			result.Status = "partial"
			result.Message = "Some test executions failed or did not produce coverage profiles."
			break
		}
	}
	return result, nil
}

func appleTools(ctx context.Context) (map[string]string, error) {
	paths := map[string]string{}
	for _, name := range []string{"clang", "clang++", "llvm-profdata", "llvm-cov"} {
		var output bytes.Buffer
		if err := command(ctx, "", "", &output, "xcrun", "--find", name); err != nil {
			return nil, fmt.Errorf("find Apple toolchain executable %s: %w", name, err)
		}
		paths[name] = strings.TrimSpace(output.String())
	}
	return paths, nil
}

func runTests(ctx context.Context, work, build string, names, binaries []string) ([]Run, []string, error) {
	var runs []Run
	var profiles []string
	ctest := len(binaries) == 0
	if ctest {
		var listing bytes.Buffer
		if err := command(ctx, build, "", &listing, "ctest", "--test-dir", build, "--show-only=json-v1"); err != nil {
			return nil, nil, fmt.Errorf("discover CTest tests: %w", err)
		}
		var inventory struct {
			Tests []json.RawMessage
		}
		if err := json.Unmarshal(listing.Bytes(), &inventory); err != nil {
			return nil, nil, fmt.Errorf("read CTest inventory: %w", err)
		}
		if len(inventory.Tests) == 0 {
			return []Run{{Name: "ctest", Status: "no-tests", Message: "No tests are registered with CTest; supply --test-binary for a built executable."}}, nil, nil
		}
		names, binaries = []string{"ctest"}, []string{"ctest"}
	}
	for index, binary := range binaries {
		profileDir := filepath.Join(work, fmt.Sprintf("profiles-%d", index))
		if err := os.Mkdir(profileDir, 0700); err != nil {
			return nil, nil, err
		}
		var args []string
		dir := filepath.Dir(binary)
		if ctest {
			dir = build
			args = []string{"--test-dir", build, "--output-on-failure", "--no-tests=error"}
		}
		err := command(ctx, dir, filepath.Join(profileDir, "%m-%p.profraw"), nil, binary, args...)
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		found, globErr := filepath.Glob(filepath.Join(profileDir, "*.profraw"))
		if globErr != nil {
			return nil, nil, globErr
		}
		run := Run{Name: names[index], Status: "succeeded", Profiles: len(found)}
		if err != nil {
			run.Status, run.Message = "failed", err.Error()
		} else if len(found) == 0 {
			run.Status = "no-profile"
		}
		runs, profiles = append(runs, run), append(profiles, found...)
	}
	return runs, profiles, nil
}

func exportLLVM(ctx context.Context, work, root string, tools map[string]string, profiles, objects []string) (Evidence, error) {
	merged := filepath.Join(work, "coverage.profdata")
	merge := append([]string{"merge", "-sparse", "-o", merged}, profiles...)
	if err := command(ctx, work, "", nil, tools["llvm-profdata"], merge...); err != nil {
		return Evidence{}, fmt.Errorf("merge coverage profiles: %w", err)
	}
	artifact := filepath.Join(work, "llvm-cov.json")
	output, err := os.Create(artifact)
	if err != nil {
		return Evidence{}, err
	}
	args := []string{"export", "-instr-profile", merged, "-skip-expansions", "-skip-functions", objects[0]}
	for _, object := range objects[1:] {
		args = append(args, "-object", object)
	}
	err = command(ctx, work, "", output, tools["llvm-cov"], args...)
	if err := errors.Join(err, output.Close()); err != nil {
		return Evidence{}, fmt.Errorf("export LLVM coverage: %w", err)
	}
	evidence, err := ImportLLVM(ctx, root, artifact)
	evidence.Artifact = "llvm-cov.json"
	evidence.Notes = append(evidence.Notes, "Generated by an instrumented Debug build in this invocation. The working tree was not snapshotted.")
	return evidence, err
}

type logTail struct{ data []byte }

func (l *logTail) Write(data []byte) (int, error) {
	const limit = 16 << 10
	length := len(data)
	if length >= limit {
		l.data = append(l.data[:0], data[length-limit:]...)
	} else {
		l.data = append(l.data, data...)
		if len(l.data) > limit {
			l.data = l.data[:copy(l.data, l.data[len(l.data)-limit:])]
		}
	}
	return length, nil
}

func command(ctx context.Context, dir, profile string, output io.Writer, program string, args ...string) error {
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Dir, cmd.WaitDelay = dir, 3*time.Second
	process.ConfigureCancellation(cmd)
	if profile != "" {
		cmd.Env = append(cmd.Environ(), "LLVM_PROFILE_FILE="+profile)
	}
	log := &logTail{}
	cmd.Stdout, cmd.Stderr = log, log
	if output != nil {
		cmd.Stdout = output
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		message := strings.TrimSpace(string(log.data))
		if message != "" {
			return fmt.Errorf("%s: %w\n%s", filepath.Base(program), err, message)
		}
		return fmt.Errorf("%s: %w", filepath.Base(program), err)
	}
	return nil
}
