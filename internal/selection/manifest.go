// Package selection consumes Core-owned, immutable file lists. It does not
// interpret project configuration, Git state or language-specific build rules.
package selection

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Reference struct {
	FileCount int    `json:"fileCount"`
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
}
type Receipt struct {
	SHA256    string `json:"sha256"`
	FileCount int    `json:"fileCount"`
}
type Set struct {
	Files     []string
	Receipt   *Receipt
	root      *os.Root
	directory string
}

func Load(directory string, reference *Reference) (*Set, error) {
	if reference == nil {
		return nil, nil
	}
	if !filepath.IsAbs(reference.Path) {
		return nil, fmt.Errorf("file selection path must be absolute")
	}
	file, err := os.Open(reference.Path)
	if err != nil {
		return nil, err
	}
	const limit = 64 << 20
	data, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(data) > limit {
		return nil, fmt.Errorf("file selection exceeds 64 MiB")
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	if reference.SHA256 != digest {
		return nil, fmt.Errorf("file selection digest mismatch")
	}
	var manifest struct {
		SchemaVersion int      `json:"schemaVersion"`
		Root          string   `json:"root"`
		Files         []string `json:"files"`
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	if manifest.SchemaVersion != 1 || manifest.Root != directory || manifest.Files == nil || len(manifest.Files) != reference.FileCount {
		return nil, fmt.Errorf("invalid file selection version, root or file list")
	}
	seen := map[string]bool{}
	for _, name := range manifest.Files {
		if !filepath.IsLocal(filepath.FromSlash(name)) || name == "." || strings.Contains(name, "\\") || filepath.ToSlash(filepath.Clean(filepath.FromSlash(name))) != name || seen[name] {
			return nil, fmt.Errorf("invalid or duplicate selected path %q", name)
		}
		for _, part := range strings.Split(name, "/") {
			if part == ".git" {
				return nil, fmt.Errorf("Git metadata cannot be selected")
			}
		}
		seen[name] = true
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	return &Set{Files: manifest.Files, Receipt: &Receipt{SHA256: digest, FileCount: len(manifest.Files)}, root: root, directory: directory}, nil
}
func (set *Set) Close() error {
	if set == nil {
		return nil
	}
	return set.root.Close()
}
func (set *Set) ReadFile(relative string) ([]byte, error) {
	file, err := set.root.Open(filepath.FromSlash(relative))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}
func (set *Set) Walk(ctx context.Context, visit fs.WalkDirFunc) error {
	for _, name := range set.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := set.root.Lstat(filepath.FromSlash(name))
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("selected path is no longer a regular file: %s", name)
		}
		if err = visit(filepath.Join(set.directory, filepath.FromSlash(name)), fs.FileInfoToDirEntry(info), nil); err != nil {
			return err
		}
	}
	return nil
}
