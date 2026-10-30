package deletionguard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// removalCommands are the commands whose effect is to unlink stored data.
var removalCommands = []string{"rm", "rmdir", "unlink", "shred", "srm", "wipe"}

// Classify reports the effect of a shell command on stored data. The command is
// parsed, never executed. An unrecognized command has EffectNone; a recognized
// removal always reports EffectPermanentDeletion even when its targets cannot
// be proven, because an unproven target set can never satisfy the gate.
func Classify(command, workingDir string) Classification {
	file, err := syntax.NewParser().Parse(strings.NewReader(command), "")
	if err != nil {
		// The interpreter rejects an unparsable command before it has any effect.
		return Classification{Effect: EffectNone, Reason: "the shell cannot parse the command"}
	}
	c := &classifier{workingDir: workingDir}
	dir := workingDir
	for _, stmt := range file.Stmts {
		c.statement(stmt, &dir)
	}
	return c.result()
}

// classifier collects the effects of one parsed command line.
type classifier struct {
	workingDir string
	changedDir bool
	effects    []Classification
}

func (c *classifier) add(effect Classification) {
	if effect.Effect == EffectNone {
		return
	}
	c.effects = append(c.effects, effect)
}

func (c *classifier) result() Classification {
	var out Classification
	for _, effect := range c.effects {
		if effect.Effect > out.Effect {
			out.Effect = effect.Effect
		}
		out.Recursive = out.Recursive || effect.Recursive
		if out.Unresolved == "" {
			out.Unresolved = effect.Unresolved
		}
		out.Targets = append(out.Targets, effect.Targets...)
		out.Operations = mergeOperations(out.Operations, effect.Operations)
	}
	for _, effect := range c.effects {
		if effect.Effect != out.Effect {
			continue
		}
		if out.Command == "" {
			out.Command = effect.Command
		}
		if out.Reason == "" {
			out.Reason = effect.Reason
		}
	}
	out.Targets = CanonicalTargets(out.Targets)
	if out.Effect == EffectPermanentDeletion && c.changedDir && out.Unresolved == "" {
		out.Unresolved = "the command changes directory first, so the removed paths are not provable"
	}
	return out
}

// statement walks one statement and updates dir with any directory change that
// affects later statements in the same list.
func (c *classifier) statement(stmt *syntax.Stmt, dir *string) {
	if stmt == nil {
		return
	}
	if stmt.Background {
		start := len(c.effects)
		defer func() {
			for i := start; i < len(c.effects); i++ {
				if c.effects[i].IsDestructive() {
					c.effects[i].Unresolved = "the destructive shell statement runs asynchronously, so its completion cannot be proven"
				}
			}
		}()
	}
	c.redirect(stmt, *dir)
	for _, redir := range stmt.Redirs {
		c.nestedStatements(redir, *dir)
	}
	switch command := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		c.nestedStatements(command, *dir)
		c.call(command, dir)
	case *syntax.BinaryCmd:
		if command.Op == syntax.AndStmt {
			c.statement(command.X, dir)
			c.statement(command.Y, dir)
			return
		}
		// Pipes and or-lists may skip either side, so neither keeps a
		// provable working directory.
		unknown := ""
		c.statement(command.X, &unknown)
		unknown = ""
		c.statement(command.Y, &unknown)
		*dir = ""
	case *syntax.Block:
		for _, child := range command.Stmts {
			c.statement(child, dir)
		}
	case *syntax.Subshell:
		nested := *dir
		for _, child := range command.Stmts {
			c.statement(child, &nested)
		}
	case *syntax.IfClause, *syntax.WhileClause, *syntax.ForClause,
		*syntax.CaseClause, *syntax.FuncDecl, *syntax.ArithmCmd,
		*syntax.TestClause, *syntax.DeclClause, *syntax.LetClause,
		*syntax.TimeClause, *syntax.CoprocClause, *syntax.TestDecl:
		// Visit every branch and expansion. We do not predict runtime branch
		// selection or loop values; nonliteral deletion targets fail closed.
		c.nestedStatements(command, *dir)
		*dir = ""
	case nil:
		// A statement containing only redirects has no command body.
	default:
		c.unresolved("the shell structure cannot be inspected")
	}
}

// nestedStatements uses the parser's complete visitor so branch conditions,
// loop bodies, function bodies and substitutions cannot hide executable code.
// Each child statement is owned by statement, avoiding duplicate traversal.
func (c *classifier) nestedStatements(node syntax.Node, dir string) {
	syntax.Walk(node, func(child syntax.Node) bool {
		if stmt, ok := child.(*syntax.Stmt); ok {
			nested := dir
			c.statement(stmt, &nested)
			return false
		}
		return true
	})
}

func (c *classifier) unresolved(reason string) {
	c.add(Classification{
		Effect:     EffectPermanentDeletion,
		Unresolved: reason,
		Reason:     "The command may permanently delete data, and " + reason + ".",
	})
}

// redirect classifies a truncating redirect. Only ">" and ">|" replace existing
// content; ">>" appends and keeps the prior bytes.
func (c *classifier) redirect(stmt *syntax.Stmt, dir string) {
	for _, redir := range stmt.Redirs {
		if redir.Op != syntax.RdrOut && redir.Op != syntax.ClbOut {
			continue
		}
		name, ok := literalWord(redir.Word)
		if !ok || name == "" {
			c.unresolved("the redirect target is not literal, so its contents cannot be protected")
			continue
		}
		if name == "/dev/null" {
			continue
		}
		target, err := c.resolve(dir, name)
		if err != nil || !destroysContent(target) {
			// Creating a file or redirecting to a device keeps stored data.
			continue
		}
		producer := *stmt
		producer.Redirs = nil
		for _, input := range stmt.Redirs {
			if input.Op != syntax.RdrOut && input.Op != syntax.ClbOut {
				producer.Redirs = append(producer.Redirs, input)
			}
		}
		var printed strings.Builder
		if err := syntax.NewPrinter().Print(&printed, &producer); err != nil {
			c.unresolved("the redirect producer cannot be inspected")
			continue
		}
		descriptor, _ := json.Marshal([][]string{{redir.Op.String(), printed.String()}})
		c.add(Classification{
			Effect:     EffectPermanentDeletion,
			Command:    ">",
			Targets:    []string{target},
			Operations: map[string]string{target: string(descriptor)},
			Reason:     "The redirect truncates existing content in " + target + ".",
		})
	}
}

// destroysContent reports an existing regular file with bytes to lose.
func destroysContent(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Size() > 0
}

func (c *classifier) call(command *syntax.CallExpr, dir *string) {
	var args []string
	complete := true
	for _, word := range command.Args {
		value, ok := literalWord(word)
		if !ok {
			complete = false
			break
		}
		args = append(args, value)
	}
	if len(args) == 0 {
		if !complete {
			c.unresolved("the command name is not literal, so its effect cannot be proven")
			*dir = ""
		}
		return
	}
	if args[0] == "cd" {
		if !complete {
			c.changedDir = true
			*dir = ""
		} else {
			*dir = c.changeDirectory(args, *dir)
		}
		return
	}
	c.command(args, complete, *dir)
}

// changeDirectory returns the directory later statements run in, or an empty
// string when it cannot be proven.
func (c *classifier) changeDirectory(args []string, dir string) string {
	if len(args) != 2 || dir == "" {
		c.changedDir = true
		return ""
	}
	next := args[1]
	if !filepath.IsAbs(next) {
		next = filepath.Join(dir, next)
	}
	next = filepath.Clean(next)
	c.changedDir = true
	// A relative cd may still fail at run time, so the resolved directory is
	// used for classification while result() records that it is not provable.
	return next
}

func (c *classifier) command(args []string, complete bool, dir string) {
	name := filepath.Base(args[0])
	// Wrapper arguments may belong to a nested script rather than the
	// wrapper itself, so a later --help must not hide that script.
	if hasInformationalFlag(args[1:]) && !slices.Contains(
		[]string{"sudo", "doas", "nohup", "time", "command", "builtin", "exec", "eval", "sh", "bash", "dash", "zsh", "ksh"}, name) {
		return
	}
	if slices.Contains(removalCommands, name) || slices.Contains([]string{"truncate", "dd", "find", "git", "mv", "rsync"}, name) {
		start := len(c.effects)
		defer func() {
			for i := start; i < len(c.effects); i++ {
				effect := &c.effects[i]
				if effect.Effect != EffectPermanentDeletion {
					continue
				}
				effect.Operations = make(map[string]string, len(effect.Targets))
				descriptor := operationArguments(args, effect.Targets, dir)
				data, _ := json.Marshal([][]string{descriptor})
				for _, target := range effect.Targets {
					effect.Operations[target] = string(data)
				}
			}
		}()
	}
	if slices.Contains(removalCommands, name) {
		flags, paths := operands(args)
		c.removal(name, paths, recursiveFlag(flags), complete, dir)
		return
	}
	switch name {
	case "sudo", "doas", "nohup", "time":
		if len(args) > 1 {
			c.command(args[1:], complete, dir)
		}
	case "command", "builtin", "exec":
		nested := args[1:]
		if name == "command" && len(nested) > 0 && (nested[0] == "-v" || nested[0] == "-V") {
			return
		}
		if len(nested) > 0 && nested[0] == "--" {
			nested = nested[1:]
		}
		if len(nested) > 0 && !strings.HasPrefix(nested[0], "-") {
			c.command(nested, complete, dir)
		} else if !complete || len(nested) > 0 {
			c.unresolved("the wrapped command cannot be inspected")
		}
	case "cd":
		// A directory change hidden behind a builtin wrapper still affects
		// the target identity of later commands.
		c.changedDir = true
	case "eval":
		if !complete {
			c.unresolved("the evaluated script is not literal")
			return
		}
		script, err := syntax.NewParser().Parse(strings.NewReader(strings.Join(args[1:], " ")), "")
		if err != nil {
			c.unresolved("the evaluated script cannot be parsed")
			return
		}
		for _, stmt := range script.Stmts {
			c.statement(stmt, &dir)
		}
	case "xargs":
		c.xargs(args, dir)
	case "sh", "bash", "dash", "zsh", "ksh":
		c.shellCommand(args, complete, dir)
	case "truncate":
		c.truncate(args, complete, dir)
	case "dd":
		for _, arg := range args[1:] {
			if value, ok := strings.CutPrefix(arg, "of="); ok && value != "" && value != "/dev/null" {
				c.removal("dd", []string{value}, false, complete, dir)
			}
		}
	case "find":
		c.find(args, complete, dir)
	case "git":
		c.git(args, complete, dir)
	case "mv":
		c.move(args, complete, dir)
	case "rsync":
		if hasDeleteFlag(args[1:]) {
			_, paths := operands(args)
			c.removal("rsync", paths, true, complete, dir)
		}
	}
}

// removal records a permanent deletion of the given operand paths.
func (c *classifier) removal(command string, paths []string, recursive, complete bool, dir string) {
	effect := Classification{Effect: EffectPermanentDeletion, Command: command, Recursive: recursive}
	switch {
	case !complete:
		effect.Unresolved = "the command arguments are not literal, so the exact targets cannot be proven"
	case len(paths) == 0:
		effect.Unresolved = "the command names no target"
	}
	for _, name := range paths {
		target, err := c.resolve(dir, name)
		if err != nil {
			if effect.Unresolved == "" {
				effect.Unresolved = "a target path could not be resolved"
			}
			continue
		}
		effect.Targets = append(effect.Targets, target)
	}
	if effect.Unresolved == "" {
		effect.Reason = "The command " + command + " permanently deletes " + strings.Join(effect.Targets, ", ") + "."
	} else {
		effect.Reason = "The command " + command + " permanently deletes data, and " + effect.Unresolved + "."
	}
	c.add(effect)
}

// move classifies a move as quarantine. A move onto an existing file destroys
// that file's content, so only the overwritten destination is permanent.
func (c *classifier) move(args []string, complete bool, dir string) {
	_, paths := operands(args)
	if len(paths) < 2 {
		return
	}
	destination := paths[len(paths)-1]
	sources := paths[:len(paths)-1]
	if !complete {
		c.add(Classification{
			Effect:     EffectQuarantine,
			Command:    "mv",
			Unresolved: "the move arguments are not literal",
			Reason:     "The command moves data; a later move can restore it.",
		})
		return
	}
	target, err := c.resolve(dir, destination)
	if err != nil {
		c.add(Classification{
			Effect:     EffectQuarantine,
			Command:    "mv",
			Unresolved: "the move destination could not be resolved",
			Reason:     "The command moves data; a later move can restore it.",
		})
		return
	}
	if info, statErr := os.Lstat(target); statErr == nil && !info.IsDir() {
		c.add(Classification{
			Effect:  EffectPermanentDeletion,
			Command: "mv",
			Targets: []string{target},
			Reason:  "The move replaces the existing file " + target + ".",
		})
		return
	}
	var targets []string
	for _, source := range sources {
		resolved, err := c.resolve(dir, source)
		if err != nil {
			continue
		}
		targets = append(targets, resolved)
	}
	c.add(Classification{
		Effect:  EffectQuarantine,
		Command: "mv",
		Targets: targets,
		Reason:  "The command moves data to " + target + "; a later move can restore it.",
	})
}

// find classifies "find -delete" and "-exec" forms. The matched set cannot be
// enumerated statically, so the target set stays unresolved.
func (c *classifier) find(args []string, complete bool, dir string) {
	destructive := false
	for i, arg := range args {
		switch arg {
		case "-delete":
			destructive = true
		case "-exec", "-execdir", "-ok", "-okdir":
			if i+1 < len(args) && slices.Contains(removalCommands, filepath.Base(args[i+1])) {
				destructive = true
			}
		}
	}
	if !destructive {
		return
	}
	var roots []string
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-") || arg == "(" || arg == "!" {
			break
		}
		roots = append(roots, arg)
	}
	if len(roots) == 0 {
		roots = []string{"."}
	}
	effect := Classification{Effect: EffectPermanentDeletion, Command: "find", Recursive: true}
	if !complete {
		effect.Unresolved = "the find arguments are not literal, so the exact targets cannot be proven"
	}
	for _, root := range roots {
		target, err := c.resolve(dir, root)
		if err != nil {
			continue
		}
		effect.Targets = append(effect.Targets, target)
	}
	if effect.Unresolved == "" {
		effect.Unresolved = "find removes every match under " + strings.Join(effect.Targets, ", ") + ", so the exact targets cannot be proven"
	}
	effect.Reason = "The command find permanently deletes data, and " + effect.Unresolved + "."
	c.add(effect)
}

// git classifies the git subcommands that destroy working-tree data.
func (c *classifier) git(args []string, complete bool, dir string) {
	if len(args) < 2 {
		return
	}
	switch args[1] {
	case "rm":
		_, paths := operands(args[1:])
		c.removal("git rm", paths, false, complete, dir)
	case "clean":
		if hasDryRunFlag(args[1:]) {
			return
		}
		c.add(Classification{
			Effect:     EffectPermanentDeletion,
			Command:    "git clean",
			Recursive:  true,
			Unresolved: "git clean removes untracked files that cannot be listed before it runs",
			Reason:     "The command git clean permanently deletes untracked files, and their exact set cannot be proven.",
		})
	case "reset":
		if !slices.Contains(args[1:], "--hard") {
			return
		}
		c.add(Classification{
			Effect:     EffectPermanentDeletion,
			Command:    "git reset --hard",
			Recursive:  true,
			Unresolved: "git reset --hard discards every uncommitted change in the working tree",
			Reason:     "The command git reset --hard permanently deletes uncommitted work, and the exact set cannot be proven.",
		})
	}
}

// truncate classifies a size change that discards existing content. The size
// value is an argument of -s and is not a target.
func (c *classifier) truncate(args []string, complete bool, dir string) {
	var paths []string
	options := true
	skipValue := false
	for _, arg := range args[1:] {
		switch {
		case skipValue:
			skipValue = false
		case options && arg == "--":
			options = false
		case options && (arg == "-s" || arg == "--size"):
			skipValue = true
		case options && strings.HasPrefix(arg, "-"):
			// "-s0", "--size=0" and every other flag carry no target.
		default:
			paths = append(paths, arg)
		}
	}
	c.removal("truncate", paths, false, complete, dir)
}

// xargs classifies a pipeline that runs a removal command over generated
// arguments.
func (c *classifier) xargs(args []string, dir string) {
	for i := 1; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			continue
		}
		if !slices.Contains(removalCommands, filepath.Base(args[i])) {
			return
		}
		_, paths := operands(args[i:])
		effect := Classification{
			Effect:     EffectPermanentDeletion,
			Command:    "xargs " + args[i],
			Unresolved: "xargs builds its target list from standard input, so the exact targets cannot be proven",
			Reason:     "The pipeline permanently deletes data, and its exact targets cannot be proven.",
		}
		for _, name := range paths {
			if target, err := c.resolve(dir, name); err == nil {
				effect.Targets = append(effect.Targets, target)
			}
		}
		c.add(effect)
		return
	}
}

// shellCommand classifies a nested "-c" script so the wrapper does not hide a
// deletion.
func (c *classifier) shellCommand(args []string, complete bool, dir string) {
	if !complete {
		c.unresolved("the nested shell script cannot be inspected")
		return
	}
	for i := 1; i < len(args); i++ {
		if !strings.HasPrefix(args[i], "-") || strings.HasPrefix(args[i], "--") || !strings.ContainsRune(args[i][1:], 'c') || i+1 >= len(args) {
			continue
		}
		nestedDir := dir
		if nestedDir == "" {
			nestedDir = c.workingDir
		}
		nested := Classify(args[i+1], nestedDir)
		if nested.IsDestructive() {
			c.add(nested)
		}
		return
	}
}

func (c *classifier) resolve(dir, name string) (string, error) {
	base := dir
	if base == "" {
		base = c.workingDir
	}
	return ResolveTarget(base, name)
}

// operands splits arguments into flags and path operands. Argument zero is the
// command name.
func operands(args []string) (flags, paths []string) {
	options := true
	for _, arg := range args[1:] {
		switch {
		case options && arg == "--":
			options = false
		case options && strings.HasPrefix(arg, "-") && len(arg) > 1:
			flags = append(flags, arg)
		case arg != "":
			paths = append(paths, arg)
		}
	}
	return flags, paths
}

// recursiveFlag reports a recursive removal among short or long flags.
func recursiveFlag(flags []string) bool {
	for _, flag := range flags {
		if strings.HasPrefix(flag, "--") {
			if flag == "--recursive" {
				return true
			}
			continue
		}
		if strings.ContainsAny(flag[1:], "rR") {
			return true
		}
	}
	return false
}

// hasInformationalFlag reports a help or version request, which has no effect.
func hasInformationalFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "--version" {
			return true
		}
	}
	return false
}

// hasDryRunFlag reports a documented dry run for git clean. Short flags may be
// clustered, as in "-nd".
func hasDryRunFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--dry-run" {
			return true
		}
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && strings.ContainsRune(arg[1:], 'n') {
			return true
		}
	}
	return false
}

// hasDeleteFlag reports rsync's destination-side deletion.
func hasDeleteFlag(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "--delete", "--delete-before", "--delete-during", "--delete-after", "--delete-delay", "--delete-excluded":
			return true
		}
	}
	return false
}

// literalWord returns a word's text when every part is literal and free of
// expansion, glob and tilde syntax.
func literalWord(word *syntax.Word) (string, bool) {
	if word == nil {
		return "", false
	}
	var text strings.Builder
	for _, part := range word.Parts {
		switch value := part.(type) {
		case *syntax.Lit:
			if strings.Contains(value.Value, `\`) || strings.ContainsAny(value.Value, "*?[~{") {
				return "", false
			}
			text.WriteString(value.Value)
		case *syntax.SglQuoted:
			text.WriteString(value.Value)
		case *syntax.DblQuoted:
			inner, ok := literalWord(&syntax.Word{Parts: value.Parts})
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

// operationArguments retains the executable and destructive options while
// replacing only target operands. Thus retrying a remainder keeps its identity
// while changing truncate's size or adding another destructive command does not.
func operationArguments(args, targets []string, dir string) []string {
	out := []string{args[0]}
	name := filepath.Base(args[0])
	// Rsync operands have different source/destination roles. Retaining them
	// prevents swapping those roles from reusing the same authorization.
	if name == "rsync" {
		return append(out, args[1:]...)
	}
	value := false
	for index, arg := range args[1:] {
		if name == "git" && index == 0 {
			out = append(out, arg)
			continue
		}
		if value {
			out = append(out, arg)
			value = false
			continue
		}
		if strings.HasPrefix(arg, "-") {
			out = append(out, arg)
			if name == "truncate" && (arg == "-s" || arg == "--size" || arg == "-r" || arg == "--reference") {
				value = true
			}
			if name == "shred" && (arg == "-n" || arg == "--iterations" || arg == "-s" || arg == "--size" || arg == "--random-source") {
				value = true
			}
			continue
		}
		candidate := arg
		if name == "dd" && strings.HasPrefix(arg, "of=") {
			candidate = strings.TrimPrefix(arg, "of=")
		}
		resolved, err := ResolveTarget(dir, candidate)
		if err == nil && slices.Contains(targets, resolved) {
			continue
		}
		out = append(out, arg)
	}
	return out
}
