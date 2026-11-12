package ipc

import (
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

type pipeAccess struct {
	AccessMode  string `json:"accessMode,omitempty"`
	AccessFlags string `json:"accessFlags,omitempty"`
}

func readPipeAccess(api string, argument *sitter.Node, source []byte) pipeAccess {
	access := pipeAccess{AccessMode: "unknown", AccessFlags: argument.Utf8Text(source)}
	flags, ok := pipeAccessFlags(argument, source)
	if !ok {
		return access
	}

	server := strings.HasPrefix(api, "CreateNamedPipe")
	var read, write bool
	for _, flag := range flags {
		switch {
		case server && flag == "PIPE_ACCESS_DUPLEX":
			read, write = true, true
		case server && flag == "PIPE_ACCESS_INBOUND", !server && flag == "GENERIC_READ":
			read = true
		case server && flag == "PIPE_ACCESS_OUTBOUND", !server && flag == "GENERIC_WRITE":
			write = true
		case server && isPipeOpenOption(flag):
			continue
		default:
			return access
		}
	}
	switch {
	case read && write:
		access.AccessMode = "read-write"
	case read:
		access.AccessMode = "read"
	case write:
		access.AccessMode = "write"
	}
	return access
}

func isPipeOpenOption(flag string) bool {
	switch flag {
	case "FILE_FLAG_FIRST_PIPE_INSTANCE", "FILE_FLAG_OVERLAPPED", "FILE_FLAG_WRITE_THROUGH",
		"WRITE_DAC", "WRITE_OWNER", "ACCESS_SYSTEM_SECURITY":
		return true
	default:
		return false
	}
}

func pipeAccessFlags(node *sitter.Node, source []byte) ([]string, bool) {
	if node == nil {
		return nil, false
	}
	switch node.Kind() {
	case "identifier":
		return []string{node.Utf8Text(source)}, true
	case "parenthesized_expression":
		for i := uint(0); i < node.NamedChildCount(); i++ {
			if child := node.NamedChild(i); child.Kind() != "comment" {
				return pipeAccessFlags(child, source)
			}
		}
	case "binary_expression":
		operator := node.ChildByFieldName("operator")
		if operator != nil && operator.Kind() == "|" {
			left, leftOK := pipeAccessFlags(node.ChildByFieldName("left"), source)
			right, rightOK := pipeAccessFlags(node.ChildByFieldName("right"), source)
			return append(left, right...), leftOK && rightOK
		}
	}
	return nil, false
}
