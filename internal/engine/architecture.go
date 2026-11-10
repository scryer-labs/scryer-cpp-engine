package engine

import (
	"slices"
	"strconv"

	"github.com/scryer-labs/scryer-cpp-engine/internal/cmake"
)

type component struct {
	ID string `json:"componentId"`
	cmake.Target
	Aliases []string `json:"aliases,omitempty"`
}

type componentLink struct {
	cmake.Link
	FromIDs []string `json:"fromComponentIds,omitempty"`
	ToIDs   []string `json:"toComponentIds,omitempty"`
}

type componentAlias struct {
	cmake.Target
	ComponentIDs []string `json:"componentIds,omitempty"`
}

type architecture struct {
	ProjectID         string             `json:"projectId"`
	Basis             string             `json:"basis"`
	AliasDeclarations []componentAlias   `json:"aliasDeclarations"`
	Components        []component        `json:"components"`
	Links             []componentLink    `json:"links"`
	Unresolved        []cmake.Unresolved `json:"unresolved"`
}

func componentsByName(targets []cmake.Target) map[string][]string {
	targetsByName := make(map[string][]cmake.Target)
	for _, target := range targets {
		targetsByName[target.Name] = append(targetsByName[target.Name], target)
	}
	var canonical func(string, map[string]bool) []string
	canonical = func(name string, visiting map[string]bool) []string {
		if visiting[name] {
			return nil
		}
		visiting[name] = true
		defer delete(visiting, name)
		var ids []string
		for _, target := range targetsByName[name] {
			if target.Type != "ALIAS" {
				ids = append(ids, componentID(target))
				continue
			}
			for _, id := range canonical(target.AliasOf, visiting) {
				if !slices.Contains(ids, id) {
					ids = append(ids, id)
				}
			}
		}
		return ids
	}

	index := make(map[string][]string, len(targetsByName))
	for name := range targetsByName {
		index[name] = canonical(name, map[string]bool{})
	}
	return index
}

func componentID(target cmake.Target) string {
	return target.File + ":" + strconv.Itoa(target.Line) + ":" + target.Name
}

func describeArchitecture(root string, facts cmake.Facts) architecture {
	index := componentsByName(facts.Targets)
	result := architecture{
		ProjectID: root, Basis: "cmake-declarations",
		Components: make([]component, 0, len(facts.Targets)),
		Links:      make([]componentLink, 0, len(facts.Links)),
		Unresolved: facts.Unresolved, AliasDeclarations: []componentAlias{},
	}
	aliases := make(map[string][]string)
	for _, target := range facts.Targets {
		if target.Type != "ALIAS" {
			continue
		}
		ids := index[target.AliasOf]
		result.AliasDeclarations = append(result.AliasDeclarations, componentAlias{Target: target, ComponentIDs: ids})
		if len(ids) == 1 && !slices.Contains(aliases[ids[0]], target.Name) {
			aliases[ids[0]] = append(aliases[ids[0]], target.Name)
		}
	}
	for _, target := range facts.Targets {
		if target.Type != "ALIAS" {
			id := componentID(target)
			result.Components = append(result.Components, component{ID: id, Target: target, Aliases: aliases[id]})
		}
	}
	for _, link := range facts.Links {
		result.Links = append(result.Links, componentLink{Link: link, FromIDs: index[link.From], ToIDs: index[link.To]})
	}
	return result
}
