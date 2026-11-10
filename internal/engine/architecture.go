package engine

import (
	"strconv"

	"github.com/scryer-labs/scryer-cpp-engine/internal/cmake"
)

type component struct {
	ID string `json:"componentId"`
	cmake.Target
	AliasOfIDs []string `json:"aliasOfComponentIds,omitempty"`
}

type componentLink struct {
	cmake.Link
	FromIDs []string `json:"fromComponentIds,omitempty"`
	ToIDs   []string `json:"toComponentIds,omitempty"`
}

type architecture struct {
	ProjectID  string             `json:"projectId"`
	Basis      string             `json:"basis"`
	Components []component        `json:"components"`
	Links      []componentLink    `json:"links"`
	Unresolved []cmake.Unresolved `json:"unresolved"`
}

func componentsByName(targets []cmake.Target) map[string][]string {
	index := make(map[string][]string)
	for _, target := range targets {
		index[target.Name] = append(index[target.Name], componentID(target))
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
		Unresolved: facts.Unresolved,
	}
	for _, target := range facts.Targets {
		result.Components = append(result.Components, component{ID: componentID(target), Target: target, AliasOfIDs: index[target.AliasOf]})
	}
	for _, link := range facts.Links {
		result.Links = append(result.Links, componentLink{Link: link, FromIDs: index[link.From], ToIDs: index[link.To]})
	}
	return result
}
