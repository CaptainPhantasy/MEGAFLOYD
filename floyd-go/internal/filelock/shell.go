package filelock

import (
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// ShellTargets recognizes literal redirections and common file-write commands.
// Dynamic paths, arbitrary programs and complex shell control flow are not a
// complete effect analysis. They retain their normal execution behavior.
func ShellTargets(file *syntax.File, cwd string) ([]string, error) {
	var paths []string
	var firstErr error
	add := func(dir, name string) {
		if name == "" || name == "/dev/null" || dir == "" && !filepath.IsAbs(name) {
			return
		}
		path, err := Resolve(dir, name)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return
		}
		paths = append(paths, path)
	}
	var stmt func(*syntax.Stmt, *string)
	stmt = func(s *syntax.Stmt, dir *string) {
		if s == nil {
			return
		}
		for _, redir := range s.Redirs {
			switch redir.Op.String() {
			case ">", ">>", ">|", "<>", "&>", "&>>":
				if path, ok := literal(redir.Word, false); ok {
					if !filepath.IsAbs(path) && *dir != "" {
						path = filepath.Join(*dir, path)
					}
					add(*dir, path)
				}
			}
		}
		switch command := s.Cmd.(type) {
		case *syntax.CallExpr:
			var args []string
			for _, word := range command.Args {
				value, ok := literal(word, false)
				if !ok {
					if len(args) > 0 && args[0] == "cd" {
						*dir = ""
					}
					return
				}
				args = append(args, value)
			}
			if len(args) == 0 {
				return
			}
			if args[0] == "cd" {
				if len(args) != 2 || *dir == "" {
					*dir = ""
					return
				}
				next := args[1]
				if !filepath.IsAbs(next) {
					next = filepath.Join(*dir, next)
				}
				next = filepath.Clean(next)
				_, err := Resolve(*dir, next)
				if err != nil {
					*dir = ""
					return
				}
				*dir = next
				return
			}
			name := filepath.Base(args[0])
			var allowed string
			switch name {
			case "touch":
				allowed = "acm"
			case "mkdir":
				allowed = "pv"
			case "rm":
				allowed = "dfirRvI"
			case "rmdir":
				allowed = "pv"
			case "unlink":
				allowed = ""
			case "cp":
				allowed = "afipRrvnHLPX"
			case "mv":
				allowed = "finv"
			case "tee":
				allowed = "ai"
			default:
				return
			}
			var operands []string
			options := true
			for _, arg := range args[1:] {
				if options && arg == "--" {
					options = false
					continue
				}
				if options && strings.HasPrefix(arg, "-") {
					if len(arg) < 2 || strings.HasPrefix(arg, "--") {
						return
					}
					for _, flag := range arg[1:] {
						if !strings.ContainsRune(allowed, flag) {
							return
						}
					}
					continue
				}
				operands = append(operands, arg)
			}
			if name == "cp" {
				if len(operands) < 2 {
					return
				}
				operands = operands[len(operands)-1:]
			}
			for _, operand := range operands {
				add(*dir, operand)
			}
		case *syntax.BinaryCmd:
			if command.Op.String() == "&&" {
				stmt(command.X, dir)
				stmt(command.Y, dir)
			} else {
				unknown := ""
				stmt(command.X, &unknown)
				unknown = ""
				stmt(command.Y, &unknown)
				*dir = ""
			}
		case *syntax.Block:
			for _, child := range command.Stmts {
				stmt(child, dir)
			}
		case *syntax.Subshell:
			nested := *dir
			for _, child := range command.Stmts {
				stmt(child, &nested)
			}
		default:
			// A control-flow construct can change cwd in ways not known statically.
			*dir = ""
		}
	}
	for _, s := range file.Stmts {
		stmt(s, &cwd)
	}
	return paths, firstErr
}

func literal(word *syntax.Word, quoted bool) (string, bool) {
	if word == nil {
		return "", false
	}
	var text strings.Builder
	for _, part := range word.Parts {
		switch value := part.(type) {
		case *syntax.Lit:
			if strings.Contains(value.Value, `\`) || !quoted && strings.ContainsAny(value.Value, "*?[~{") {
				return "", false
			}
			text.WriteString(value.Value)
		case *syntax.SglQuoted:
			text.WriteString(value.Value)
		case *syntax.DblQuoted:
			inner, ok := literal(&syntax.Word{Parts: value.Parts}, true)
			if !ok {
				return "", false
			}
			text.WriteString(inner)
		default:
			return "", false
		}
	}
	return text.String(), true
}
