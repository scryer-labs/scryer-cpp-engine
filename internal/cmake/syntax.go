package cmake

import (
	"fmt"
	"strings"
)

type argument struct {
	text    string
	line    int
	bracket bool
}

type invocation struct {
	name string
	line int
	args []argument
}

type lexer struct {
	source string
	pos    int
	line   int
}

func (l *lexer) take() byte {
	ch := l.source[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
	}
	return ch
}

func whitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n'
}

func identifier(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_'
}

// bracketEnd recognizes both bracket arguments and the brackets after a comment marker.
func bracketEnd(source string) string {
	if len(source) < 2 || source[0] != '[' {
		return ""
	}
	i := 1
	for i < len(source) && source[i] == '=' {
		i++
	}
	if i < len(source) && source[i] == '[' {
		return "]" + source[1:i] + "]"
	}
	return ""
}

func (l *lexer) bracket(end string) (string, error) {
	line := l.line
	start := l.pos + len(end)
	offset := strings.Index(l.source[start:], end)
	if offset < 0 {
		return "", fmt.Errorf("line %d: unterminated bracket", line)
	}
	closeAt := start + offset
	text := l.source[start:closeAt]
	l.line += strings.Count(text, "\n")
	l.pos = closeAt + len(end)
	return strings.TrimPrefix(text, "\n"), nil
}

func (l *lexer) skipTrivia() error {
	for l.pos < len(l.source) {
		ch := l.source[l.pos]
		if whitespace(ch) {
			l.take()
			continue
		}
		if ch != '#' {
			break
		}
		l.take()
		if end := bracketEnd(l.source[l.pos:]); end != "" {
			if _, err := l.bracket(end); err != nil {
				return err
			}
			continue
		}
		for l.pos < len(l.source) && l.source[l.pos] != '\n' {
			l.take()
		}
	}
	return nil
}

func (l *lexer) next() (invocation, bool, error) {
	if err := l.skipTrivia(); err != nil {
		return invocation{}, false, err
	}
	if l.pos == len(l.source) {
		return invocation{}, false, nil
	}
	start := l.pos
	cmd := invocation{line: l.line}
	for l.pos < len(l.source) && identifier(l.source[l.pos]) {
		l.take()
	}
	cmd.name = strings.ToLower(l.source[start:l.pos])
	if err := l.skipTrivia(); err != nil {
		return cmd, false, err
	}
	if cmd.name == "" || l.pos == len(l.source) || l.take() != '(' {
		return cmd, false, fmt.Errorf("expected command invocation")
	}
	depth := 1
	for depth > 0 {
		if err := l.skipTrivia(); err != nil {
			return cmd, false, err
		}
		if l.pos == len(l.source) {
			return cmd, false, fmt.Errorf("unterminated %s invocation", cmd.name)
		}
		line := l.line
		switch l.source[l.pos] {
		case '(':
			depth++
			l.take()
			cmd.args = append(cmd.args, argument{text: "(", line: line})
		case ')':
			depth--
			l.take()
			if depth > 0 {
				cmd.args = append(cmd.args, argument{text: ")", line: line})
			}
		default:
			arg, err := l.argument()
			if err != nil {
				return cmd, false, err
			}
			cmd.args = append(cmd.args, arg)
		}
	}
	return cmd, true, nil
}

func (l *lexer) argument() (argument, error) {
	arg := argument{line: l.line}
	if end := bracketEnd(l.source[l.pos:]); end != "" {
		arg.bracket = true
		var err error
		arg.text, err = l.bracket(end)
		return arg, err
	}

	start := l.pos
	quoted := l.source[l.pos] == '"'
	insideQuotes := quoted
	if quoted {
		l.take()
		start++
	}
	for l.pos < len(l.source) {
		ch := l.source[l.pos]
		if quoted && ch == '"' {
			arg.text = l.source[start:l.pos]
			l.take()
			return arg, nil
		}
		if !insideQuotes && (whitespace(ch) || ch == '(' || ch == ')' || ch == '#') {
			break
		}
		l.take()
		if ch == '\\' && l.pos < len(l.source) {
			l.take()
			continue
		}
		if ch == '"' {
			insideQuotes = !insideQuotes
		}
	}
	if insideQuotes {
		return arg, fmt.Errorf("unterminated quoted argument")
	}
	arg.text = l.source[start:l.pos]
	return arg, nil
}
