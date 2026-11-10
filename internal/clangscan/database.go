package clangscan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

const databaseName = "compile_commands.json"

type command struct {
	Directory string   `json:"directory"`
	File      string   `json:"file"`
	Arguments []string `json:"arguments"`
	Command   string   `json:"command"`
}

func loadDatabase(ctx context.Context, root, explicit string) ([]command, string, func(), error) {
	cleanup := func() {}
	file, err := findDatabase(ctx, root, explicit)
	if err != nil {
		return nil, "", cleanup, err
	}
	source := "existing"
	if file == "" {
		source = "cmake-configure"
		if _, err := os.Stat(filepath.Join(root, "CMakeLists.txt")); err != nil {
			return nil, source, cleanup, fmt.Errorf("no compilation database or root CMakeLists.txt found")
		}
		directory, err := os.MkdirTemp("", "scryer-cmake-*")
		if err != nil {
			return nil, source, cleanup, err
		}
		cleanup = func() { _ = os.RemoveAll(directory) }
		if err := configure(ctx, root, directory); err != nil {
			return nil, source, cleanup, err
		}
		file = filepath.Join(directory, databaseName)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, source, cleanup, err
	}
	var commands []command
	if err := json.Unmarshal(data, &commands); err != nil {
		return nil, source, cleanup, fmt.Errorf("decode %s: %w", file, err)
	}
	for i := range commands {
		entry := &commands[i]
		if !filepath.IsAbs(entry.Directory) {
			entry.Directory = filepath.Join(filepath.Dir(file), entry.Directory)
		}
		if !filepath.IsAbs(entry.File) {
			entry.File = filepath.Join(entry.Directory, entry.File)
		}
		entry.File = filepath.Clean(entry.File)
	}
	return commands, source, cleanup, nil
}

func findDatabase(ctx context.Context, root, explicit string) (string, error) {
	if explicit != "" {
		if !filepath.IsAbs(explicit) {
			explicit = filepath.Join(root, explicit)
		}
		return explicit, nil
	}
	atRoot := filepath.Join(root, databaseName)
	if info, err := os.Stat(atRoot); err == nil && info.Mode().IsRegular() {
		return atRoot, nil
	}
	var candidates []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.IsDir() && path != root {
			switch entry.Name() {
			case ".git", "CMakeFiles", "_deps", "node_modules", ".scryer":
				return filepath.SkipDir
			}
		}
		if entry.Name() == databaseName && path != atRoot {
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
				real, err := filepath.EvalSymlinks(path)
				if err == nil {
					candidates = append(candidates, real)
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(candidates)
	unique := candidates[:0]
	for _, candidate := range candidates {
		if len(unique) == 0 || unique[len(unique)-1] != candidate {
			unique = append(unique, candidate)
		}
	}
	switch len(unique) {
	case 0:
		return "", nil
	case 1:
		return unique[0], nil
	default:
		return "", fmt.Errorf("multiple compilation databases found; select one with --compile-commands: %s", strings.Join(unique, ", "))
	}
}

func configure(ctx context.Context, root, directory string) error {
	ctx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	generator := "Unix Makefiles"
	if _, err := exec.LookPath("ninja"); err == nil {
		generator = "Ninja"
	}
	cmd := exec.CommandContext(ctx, "cmake", "-S", root, "-B", directory, "-G", generator, "-DCMAKE_EXPORT_COMPILE_COMMANDS=ON")
	cmd.Dir = directory
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 3 * time.Second
	log := &tailBuffer{}
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return fmt.Errorf("CMake configure failed: %w\n%s", err, strings.TrimSpace(log.String()))
	}
	return nil
}

type tailBuffer struct{ bytes.Buffer }

func (b *tailBuffer) Write(data []byte) (int, error) {
	const limit = 6000
	size := len(data)
	if size >= limit {
		b.Reset()
		data = data[size-limit:]
	} else if excess := b.Len() + size - limit; excess > 0 {
		b.Next(excess)
	}
	_, _ = b.Buffer.Write(data)
	return size, nil
}

func (entry command) argv() ([]string, error) {
	args := entry.Arguments
	if len(args) == 0 {
		var err error
		args, err = splitCommand(entry.Command)
		if err != nil {
			return nil, err
		}
	}
	if len(args) == 0 || args[0] == "" {
		return nil, fmt.Errorf("empty compile command")
	}
	result := []string{args[0], "-working-directory=" + entry.Directory}
	for i := 1; i < len(args); i++ {
		arg := args[i]
		// Compiler plugins and response files can alter frontend behavior beyond
		// the visible command. Keep the static facts when these are required.
		if strings.HasPrefix(arg, "@") || strings.HasPrefix(arg, "-fplugin") ||
			strings.HasPrefix(arg, "-fpass-plugin") || arg == "-load" || strings.HasPrefix(arg, "-load=") {
			return nil, fmt.Errorf("compile command contains unsupported plugin or response-file arguments")
		}
		result = append(result, arg)
	}
	return result, nil
}

// splitCommand handles quoting and escaping in CMake's command form without invoking a shell.
func splitCommand(command string) ([]string, error) {
	var args []string
	var token strings.Builder
	var quote rune
	var started, escaped bool
	for _, ch := range command {
		switch {
		case escaped:
			token.WriteRune(ch)
			escaped = false
		case ch == '\\' && quote != '\'':
			escaped, started = true, true
		case quote != 0:
			if ch == quote {
				quote = 0
			} else {
				token.WriteRune(ch)
			}
		case ch == '"' || ch == '\'':
			quote, started = ch, true
		case ch == ' ' || ch == '\t' || ch == '\n':
			if started {
				args = append(args, token.String())
				token.Reset()
				started = false
			}
		default:
			token.WriteRune(ch)
			started = true
		}
	}
	if escaped || quote != 0 {
		return nil, fmt.Errorf("unterminated quote or escape in compile command")
	}
	if started {
		args = append(args, token.String())
	}
	return args, nil
}
