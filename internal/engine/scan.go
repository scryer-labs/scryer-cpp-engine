package engine

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
)

type Include struct {
	File   string `json:"file"`
	Line   uint   `json:"line"`
	Target string `json:"target"`
	Column uint   `json:"-"`
}

type sourceFacts struct {
	files    []string
	includes []Include
}

func scanSources(ctx context.Context, root string) (sourceFacts, error) {
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(sitter.NewLanguage(cpp.Language())); err != nil {
		return sourceFacts{}, err
	}

	facts := sourceFacts{files: []string{}, includes: []Include{}}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == ".git" && path != root {
			return filepath.SkipDir
		}
		if !entry.Type().IsRegular() || !isSource(path) {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		file = filepath.ToSlash(file)
		facts.files = append(facts.files, file)
		tree := parser.ParseWithOptions(func(offset int, _ sitter.Point) []byte {
			return source[offset:]
		}, nil, &sitter.ParseOptions{ProgressCallback: func(sitter.ParseState) bool { return ctx.Err() != nil }})
		if err := ctx.Err(); err != nil {
			if tree != nil {
				tree.Close()
			}
			return err
		}
		if tree == nil {
			return fmt.Errorf("could not parse %s", file)
		}
		err = collectIncludes(ctx, tree.RootNode(), file, source, &facts.includes)
		tree.Close()
		return err
	})
	return facts, err
}

func isSource(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".c", ".cc", ".cpp", ".cxx", ".h", ".hh", ".hpp", ".hxx", ".ipp", ".inl", ".ixx", ".cppm":
		return true
	default:
		return false
	}
}

func collectIncludes(ctx context.Context, root *sitter.Node, file string, source []byte, includes *[]Include) error {
	cursor := root.Walk()
	defer cursor.Close()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		node := cursor.Node()
		if node.Kind() == "preproc_include" {
			if target := node.ChildByFieldName("path"); target != nil {
				*includes = append(*includes, Include{File: file, Line: node.StartPosition().Row + 1, Target: target.Utf8Text(source), Column: target.StartPosition().Column + 1})
			}
		}
		if cursor.GotoFirstChild() {
			continue
		}
		for !cursor.GotoNextSibling() {
			if !cursor.GotoParent() {
				return nil
			}
		}
	}
}
