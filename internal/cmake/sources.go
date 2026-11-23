package cmake

import (
	"path"
	"path/filepath"
)

// Source operands are normalized relative to the invocation's source directory,
// independently of the physical listfile containing the declaration.
func (m *model) normalizeSources(loc Location, cmd invocation) invocation {
	cmd.args = append([]argument{}, cmd.args...)
	for i, arg := range cmd.args {
		if i == 0 || !arg.literal() {
			continue
		}
		switch arg.text {
		case "STATIC", "SHARED", "MODULE", "OBJECT", "INTERFACE", "UNKNOWN", "IMPORTED", "GLOBAL", "WIN32", "MACOSX_BUNDLE", "EXCLUDE_FROM_ALL", "ALIAS", "PRIVATE", "PUBLIC":
			continue
		}
		file, ok := m.localPath(loc.SourceDirectory, arg.text)
		if !ok {
			// Retain the operand as unresolved rather than fabricate membership outside
			// the project. The remaining operands can still supply useful facts.
			m.facts.unresolved(loc, cmd, arg.text, "source path is outside the selected project")
			cmd.args[i].invalid = true
			continue
		}
		relative, err := filepath.Rel(filepath.FromSlash(loc.SourceDirectory), filepath.FromSlash(file))
		if err == nil {
			cmd.args[i].text = filepath.ToSlash(relative)
		}
	}
	return cmd
}
func (m *model) target(loc Location, cmd invocation) {
	if len(cmd.args) > 1 && (cmd.args[1].text == "ALIAS" || cmd.args[1].text == "IMPORTED") {
		m.facts.target(loc, cmd)
		return
	}
	for _, arg := range cmd.args {
		if arg.text == "IMPORTED" {
			m.facts.target(loc, cmd)
			return
		}
	}
	m.facts.target(loc, m.normalizeSources(loc, cmd))
	m.unresolvedSources(loc, cmd)
}
func (m *model) targetSources(loc Location, cmd invocation) {
	for _, arg := range cmd.args {
		if arg.text == "FILE_SET" {
			m.facts.unresolved(loc, cmd, cmd.expression(), "CMake file sets are not interpreted")
			return
		}
	}
	m.facts.targetSources(loc, m.normalizeSources(loc, cmd))
	m.unresolvedSources(loc, cmd)
}
func (m *model) unresolvedSources(loc Location, cmd invocation) {
	if len(cmd.args) == 0 || !cmd.args[0].literal() {
		return
	}
	for _, arg := range cmd.args[1:] {
		if !arg.literal() {
			loc.Line = arg.line
			m.facts.unresolved(loc, cmd, arg.text, "source operand requires evaluation")
		}
	}
}

// SourceDirectory was absent in the literal-only collector. Preserve that
// fallback for callers constructing facts directly.
func sourceDirectory(loc Location) string {
	if loc.SourceDirectory != "" {
		return loc.SourceDirectory
	}
	return path.Dir(loc.File)
}

// ProjectPath is shared by file-membership and IPC entrypoint projection.
func (source TargetSource) ProjectPath() string {
	return path.Join(sourceDirectory(source.Location), source.Path)
}
