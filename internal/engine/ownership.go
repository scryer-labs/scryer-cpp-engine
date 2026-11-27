package engine

import (
	"path"
	"slices"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

type apiDeclaration struct {
	Name       string
	Site       sourceRange
	Definition bool
}
type ownershipEvidence struct {
	Basis       string       `json:"basis"`
	ComponentID string       `json:"componentId"`
	Declaration *sourceRange `json:"declaration,omitempty"`
	Definition  *sourceRange `json:"definition,omitempty"`
	ViaFile     string       `json:"viaFile,omitempty"`
}
type headerExposure struct {
	ComponentID string `json:"componentId"`
	Visibility  string `json:"visibility"`
	Directory   string `json:"directory"`
	File        string `json:"file"`
	Line        int    `json:"line"`
}
type fileOwnership struct {
	Exposures             []headerExposure    `json:"exposures,omitempty"`
	Status                string              `json:"status"`
	CandidateComponentIDs []string            `json:"candidateComponentIds"`
	Evidence              []ownershipEvidence `json:"evidence"`
}

// Only free-function declarations/definitions are compared here. Member names,
// function-pointer variables and static helpers cannot establish header providers.
func observeAPI(node *sitter.Node, file string, source []byte) (apiDeclaration, bool) {
	if node.Kind() != "declaration" && node.Kind() != "function_definition" {
		return apiDeclaration{}, false
	}
	for parent := node.Parent(); parent != nil; parent = parent.Parent() {
		switch parent.Kind() {
		case "class_specifier", "struct_specifier", "function_definition", "lambda_expression":
			return apiDeclaration{}, false
		}
	}
	if staticFreeFunction(node, source) {
		return apiDeclaration{}, false
	}
	declarator := node.ChildByFieldName("declarator")
	for declarator != nil && declarator.Kind() != "function_declarator" {
		declarator = declarator.ChildByFieldName("declarator")
	}
	if declarator == nil {
		return apiDeclaration{}, false
	}
	name := declarator.ChildByFieldName("declarator")
	if name == nil || (name.Kind() != "identifier" && name.Kind() != "qualified_identifier") {
		return apiDeclaration{}, false
	}
	spelling := name.Utf8Text(source)
	if !strings.Contains(spelling, "::") {
		var namespaces []string
		for parent := node.Parent(); parent != nil; parent = parent.Parent() {
			if parent.Kind() == "namespace_definition" {
				if n := parent.ChildByFieldName("name"); n != nil {
					namespaces = append([]string{n.Utf8Text(source)}, namespaces...)
				}
			}
		}
		if len(namespaces) > 0 {
			spelling = strings.Join(namespaces, "::") + "::" + spelling
		}
	}
	return apiDeclaration{Name: spelling, Site: location(file, node), Definition: node.Kind() == "function_definition"}, true
}
func headerFile(file string) bool {
	switch strings.ToLower(path.Ext(file)) {
	case ".h", ".hh", ".hpp", ".hxx":
		return true
	}
	return false
}

func describeOwnership(result *filesReport, api []apiDeclaration, search *includePaths) {
	byPath := map[string]*sourceFile{}
	definitions := map[string][]apiDeclaration{}
	for i := range result.Files {
		file := &result.Files[i]
		byPath[file.Path] = file
		status := "unassigned"
		if len(file.ComponentIDs) > 0 {
			status = "declared"
		}
		file.Ownership = &fileOwnership{Status: status, CandidateComponentIDs: []string{}, Evidence: []ownershipEvidence{}}
		for _, member := range file.Memberships {
			file.Ownership.Evidence = append(file.Ownership.Evidence, ownershipEvidence{Basis: "cmake-source-list", ComponentID: member.ComponentID, ViaFile: member.File})
		}
	}
	references := map[string][]fileReference{}
	for _, ref := range result.References {
		references[ref.File] = append(references[ref.File], ref)
	}
	for i := range result.Files {
		file := &result.Files[i]
		if !headerFile(file.Path) {
			continue
		}
		for target, dirs := range search.directories {
			for _, dir := range dirs {
				if dir.Visibility != "PUBLIC" && dir.Visibility != "INTERFACE" {
					continue
				}
				if dir.Directory != "." && !strings.HasPrefix(file.Path, dir.Directory+"/") {
					continue
				}
				for _, targetDef := range search.targets[target] {
					file.Ownership.Exposures = append(file.Ownership.Exposures, headerExposure{ComponentID: componentID(targetDef), Visibility: dir.Visibility, Directory: dir.Directory, File: dir.File, Line: dir.Line})
				}
			}
		}
		slices.SortFunc(file.Ownership.Exposures, func(a, b headerExposure) int {
			return strings.Compare(a.ComponentID+a.Directory+a.File, b.ComponentID+b.Directory+b.File)
		})
	}
	for _, decl := range api {
		if decl.Definition {
			definitions[decl.Name] = append(definitions[decl.Name], decl)
		}
	}
	add := func(file *sourceFile, id string, evidence ownershipEvidence) {
		own := file.Ownership
		if !slices.Contains(own.CandidateComponentIDs, id) {
			own.CandidateComponentIDs = append(own.CandidateComponentIDs, id)
		}
		own.Evidence = append(own.Evidence, evidence)
	}
	for _, decl := range api {
		if decl.Definition || !headerFile(decl.Site.File) {
			continue
		}
		file := byPath[decl.Site.File]
		if file == nil || len(file.ComponentIDs) > 0 {
			continue
		}
		for _, definition := range definitions[decl.Name] {
			implementation := byPath[definition.Site.File]
			if implementation == nil {
				continue
			}
			for _, id := range implementation.ComponentIDs {
				declarationSite, definitionSite := decl.Site, definition.Site
				add(file, id, ownershipEvidence{Basis: "declaration-definition-name-candidate", ComponentID: id, Declaration: &declarationSite, Definition: &definitionSite})
			}
		}
	}
	// Header-only umbrellas can carry the common provider of their observed local
	// header dependencies. This remains a candidate, never declared ownership.
	for pass := 0; pass < len(result.Files); pass++ {
		changed := false
		for i := range result.Files {
			file := &result.Files[i]
			if !headerFile(file.Path) || len(file.ComponentIDs) > 0 || len(file.Ownership.CandidateComponentIDs) > 0 {
				continue
			}
			var common []string
			var providers []string
			for _, ref := range references[file.Path] {
				if ref.File != file.Path || !headerFile(ref.To) {
					continue
				}
				target := byPath[ref.To]
				if target == nil {
					continue
				}
				candidates := target.ComponentIDs
				if len(candidates) == 0 {
					candidates = target.Ownership.CandidateComponentIDs
				}
				if len(candidates) == 0 {
					continue
				}
				providers = append(providers, ref.To)
				if common == nil {
					common = append([]string{}, candidates...)
				} else {
					common = slices.DeleteFunc(common, func(id string) bool { return !slices.Contains(candidates, id) })
				}
			}
			if len(common) == 0 {
				continue
			}
			for _, id := range common {
				for _, provider := range providers {
					add(file, id, ownershipEvidence{Basis: "included-header-provider-candidate", ComponentID: id, ViaFile: provider})
				}
			}
			changed = true
		}
		if !changed {
			break
		}
	}
	for i := range result.Files {
		own := result.Files[i].Ownership
		slices.Sort(own.CandidateComponentIDs)
		if own.Status == "declared" {
			continue
		}
		switch len(own.CandidateComponentIDs) {
		case 0:
			own.Status = "unassigned"
		case 1:
			own.Status = "candidate"
		default:
			own.Status = "ambiguous"
		}
	}
}
