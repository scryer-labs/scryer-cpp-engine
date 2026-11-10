package engine

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/scryer-labs/scryer-cpp-engine/internal/clangscan"
	sitter "github.com/tree-sitter/go-tree-sitter"
	cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
)

type Include struct {
	File   string `json:"file"`
	Line   uint   `json:"line"`
	Target string `json:"target"`
	Column uint   `json:"-"`
}

type semanticFacts struct {
	Resolution *clangscan.Result `json:"resolution"`
	Edges      []clangscan.Edge  `json:"edges"`
}

type sourceFacts struct {
	resolution *clangscan.Result
	files      []string
	includes   []Include
	graph      callGraph
}

func scanSources(ctx context.Context, root string, includes, calls bool) (sourceFacts, error) {
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(sitter.NewLanguage(cpp.Language())); err != nil {
		return sourceFacts{}, err
	}

	facts := sourceFacts{files: []string{}, includes: []Include{}}
	if calls {
		facts.graph = callGraph{ProjectID: root, Basis: "tree-sitter-syntax", Symbols: []functionSymbol{}, Calls: []callSite{}}
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() && path != root {
			switch entry.Name() {
			case ".git", "CMakeFiles", "_deps":
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "CMakeCache.txt")); err == nil {
				return filepath.SkipDir
			}
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
		err = facts.collect(ctx, tree.RootNode(), file, source, includes, calls)
		tree.Close()
		return err
	})
	if err == nil && calls {
		facts.graph.findCandidates()
	}
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

func (facts *sourceFacts) collect(ctx context.Context, root *sitter.Node, file string, source []byte, includes, calls bool) error {
	cursor := root.Walk()
	defer cursor.Close()
	var scopes []functionScope
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		node := cursor.Node()
		if includes && node.Kind() == "preproc_include" {
			if target := node.ChildByFieldName("path"); target != nil {
				facts.includes = append(facts.includes, Include{File: file, Line: node.StartPosition().Row + 1, Target: target.Utf8Text(source), Column: target.StartPosition().Column + 1})
			}
		}
		if calls {
			scopes = facts.graph.collect(node, file, source, scopes)
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
