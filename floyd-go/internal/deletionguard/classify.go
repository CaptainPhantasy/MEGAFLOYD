package deletionguard

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
	"mvdan.cc/sh/v3/syntax"
)

// removalCommands are the commands whose effect is to unlink stored data.
var removalCommands = []string{"rm", "rmdir", "unlink", "shred", "srm", "wipe"}

// Classify reports the effect of a shell command on stored data. The command is
// parsed, never executed. An unrecognized command has EffectNone; a recognized
// removal always reports EffectPermanentDeletion even when its targets cannot
// be proven, because an unproven target set can never satisfy the gate.
func Classify(command, workingDir string) Classification {
	return classifyCommand(&classifier{workingDir: workingDir}, command)
}

func classifyCommand(c *classifier, command string) Classification {
	file, err := syntax.NewParser().Parse(strings.NewReader(command), "")
	if err != nil {
		// A shell may execute complete earlier lines before a later parse error.
		return Classification{Effect: EffectPermanentDeletion, Unresolved: "the shell script cannot be fully inspected because parsing failed", Reason: "An incompletely parsed script cannot establish a safe target set."}
	}
	dir := c.workingDir
	for _, stmt := range file.Stmts {
		// A proof earned inside one statement list cannot outlive it: a
		// ;-separated cd may fail while the next statement still runs in the
		// original directory. Only the caller-provided directory stays proven
		// while no cd has run.
		c.dirProven = !c.changedDir
		c.statement(stmt, &dir)
	}
	return c.result()
}

// classifier collects the effects of one parsed command line.
type classifier struct {
	workingDir string
	// changedDir reports that some cd may have run in the current scope or in
	// one enclosing it. It is restored when a nested scope ends, because a
	// subshell cannot change its caller's working directory.
	changedDir bool
	// dirProven reports that the directory relative operands currently resolve
	// against is proven: a literal cd to an existing directory ran as the left
	// side of an && chain, so the rest of that chain runs only after the cd
	// succeeded.
	dirProven bool
	// dirProofBlocked reports a scope where a cd cannot prove a directory for
	// later statements: a branch, a loop, a function body or an evaluated
	// script.
	dirProofBlocked bool
	// nested reports a scope whose statements are not the command's own: a
	// subshell, a command substitution or a nested script. What it creates
	// does not vouch for anything in the enclosing scope.
	nested bool
	// directoryOpaque reports a command that may have moved the shell without
	// the caller learning where: an evaluated script, or a directory change
	// behind a wrapper. call() empties the directory when it sees this.
	directoryOpaque bool
	// createdDirectories lists directories this command creates that were
	// absent when it was classified. Nothing stored before the command can be
	// inside them, so a later move or copy can treat one as an empty directory.
	// An entry is assumed to hold for the rest of the command: a later
	// statement that removes the directory leaves a move or copy into it
	// failing, not destructive. Statements running in the background are
	// excluded because they may not have finished.
	createdDirectories []string
	// createdLinks maps each symbolic link this command creates to the target
	// it was given, so a later statement can resolve a destination through it.
	// An empty target means the link's destination is unknown.
	createdLinks map[string]string
	// unknownLinks reports an ln in this command that creates links whose
	// names are not all known.
	unknownLinks bool
	// asynchronous reports a statement running in the background, whose
	// effects may not have happened when the next statement starts.
	asynchronous     bool
	effects          []Classification
	movePaths        []string
	destructivePaths []string
}

// directoryUnproven reports that a directory change occurred and its result is
// not proven, so relative operands cannot be trusted.
func (c *classifier) directoryUnproven() bool {
	return c.changedDir && !c.dirProven
}

func (c *classifier) add(effect Classification) {
	if effect.Effect == EffectNone {
		return
	}
	if effect.IsDestructive() && effect.Unresolved == "" && c.directoryUnproven() {
		// Record the doubt where the effect is classified: a later statement
		// that proves a different directory must not make this target look
		// provable.
		effect.Unresolved = "the command changes directory first, so the removed paths are not provable"
	}
	if effect.IsDestructive() {
		if overlappingPaths(effect.Targets, c.movePaths) {
			effect.Unresolved = "an earlier move changes a destructive target, so its original data cannot be identified safely"
		}
		c.destructivePaths = append(c.destructivePaths, effect.Targets...)
	}
	c.effects = append(c.effects, effect)
}

func overlappingPaths(one, two []string) bool {
	if overlaps(one, two) || overlaps(two, one) {
		return true
	}
	resolvedOne, resolvedTwo := statePaths(one), statePaths(two)
	if overlaps(resolvedOne, resolvedTwo) || overlaps(resolvedTwo, resolvedOne) {
		return true
	}
	// Case and Unicode aliases cannot establish disjoint state on every
	// supported filesystem. Keep literal target identities unchanged, but
	// conservatively reject these potentially overlapping move sequences.
	fold := cases.Fold()
	for i := range resolvedOne {
		resolvedOne[i] = fold.String(norm.NFC.String(resolvedOne[i]))
	}
	for i := range resolvedTwo {
		resolvedTwo[i] = fold.String(norm.NFC.String(resolvedTwo[i]))
	}
	return overlaps(resolvedOne, resolvedTwo) || overlaps(resolvedTwo, resolvedOne)
}

// statePaths resolves existing directory aliases for comparisons without
// changing the literal target names or following a final symlink to its data.
func statePaths(paths []string) []string {
	resolved := make([]string, 0, len(paths))
	for _, path := range paths {
		parent, suffix := filepath.Dir(path), filepath.Base(path)
		for {
			if canonical, err := filepath.EvalSymlinks(parent); err == nil {
				resolved = append(resolved, filepath.Join(canonical, suffix))
				break
			}
			next := filepath.Dir(parent)
			if next == parent {
				resolved = append(resolved, path)
				break
			}
			suffix, parent = filepath.Join(filepath.Base(parent), suffix), next
		}
	}
	return resolved
}

// classifyCommands keeps directory scope independent for separate tool commands
// but shares observed move effects, since all commands address one filesystem.
func classifyCommands(commands []string, workingDir string) Classification {
	combined := &classifier{workingDir: workingDir}
	for _, command := range commands {
		current := &classifier{workingDir: workingDir, movePaths: combined.movePaths, destructivePaths: combined.destructivePaths}
		combined.effects = append(combined.effects, classifyCommand(current, command))
		combined.movePaths, combined.destructivePaths = current.movePaths, current.destructivePaths
	}
	return combined.result()
}

func (c *classifier) result() Classification {
	var out Classification
	for _, effect := range c.effects {
		if effect.Effect > out.Effect {
			out.Effect = effect.Effect
		}
	}
	for _, effect := range c.effects {
		// Quarantine sources are not destructive targets and must never be
		// folded into the human authorization for a separate deletion.
		if effect.Effect != out.Effect {
			continue
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
	if out.Effect == EffectPermanentDeletion && c.directoryUnproven() && out.Unresolved == "" {
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
		start, directories, asynchronous := len(c.effects), len(c.createdDirectories), c.asynchronous
		c.asynchronous = true
		defer func() {
			c.asynchronous = asynchronous
			c.createdDirectories = c.createdDirectories[:directories]
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
			// The right-hand side runs only after a successful left side, so a
			// directory proof earned on the left carries to the right.
			c.statement(command.X, dir)
			if command.X != nil && command.X.Negated {
				// "! cd tree" succeeds when the cd fails, so the right side may
				// run in the original directory.
				c.dirProven = false
				*dir = ""
			}
			c.statement(command.Y, dir)
			return
		}
		// Pipes and or-lists may skip either side, so neither keeps a
		// provable working directory.
		unknown := ""
		c.dirProven = false
		c.statement(command.X, &unknown)
		c.dirProven = false
		unknown = ""
		c.statement(command.Y, &unknown)
		*dir = ""
		c.dirProven = false
	case *syntax.Block:
		c.statementList(command.Stmts, dir)
	case *syntax.Subshell:
		c.localDirectoryStatements(command.Stmts, *dir)
	case *syntax.IfClause, *syntax.WhileClause, *syntax.ForClause,
		*syntax.CaseClause, *syntax.FuncDecl, *syntax.ArithmCmd,
		*syntax.TestClause, *syntax.DeclClause, *syntax.LetClause,
		*syntax.TimeClause, *syntax.CoprocClause, *syntax.TestDecl:
		// Visit every branch and expansion. We do not predict runtime branch
		// selection or loop values; nonliteral deletion targets fail closed.
		// A cd inside a branch or function body cannot prove the caller's
		// directory for later statements.
		blocked := c.dirProofBlocked
		c.dirProofBlocked = true
		c.nestedStatements(command, *dir)
		c.dirProofBlocked = blocked
		*dir = ""
		c.dirProven = false
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
		switch substitution := child.(type) {
		case *syntax.CmdSubst:
			c.localDirectoryStatements(substitution.Stmts, dir)
			return false
		case *syntax.ProcSubst:
			c.localDirectoryStatements(substitution.Stmts, dir)
			return false
		}
		if stmt, ok := child.(*syntax.Stmt); ok {
			nested := dir
			c.statement(stmt, &nested)
			return false
		}
		return true
	})
}

// statementList walks statements that run in sequence whether or not the one
// before succeeded. The first runs in the directory the list was entered with,
// so it inherits that directory's proof. A later one may run after an earlier
// cd failed, so a directory proven inside the list does not carry to it.
func (c *classifier) statementList(statements []*syntax.Stmt, dir *string) {
	changed := false
	for _, statement := range statements {
		if changed {
			c.dirProven = false
		}
		before := *dir
		c.statement(statement, dir)
		changed = changed || *dir != before
	}
}

// localDirectoryStatements shares filesystem effects but restores the outer
// directory afterwards: a subshell cannot change its caller's working
// directory. The subshell inherits the caller's proven directory and may
// prove its own with a cd, for its own statements only.
func (c *classifier) localDirectoryStatements(statements []*syntax.Stmt, dir string) {
	outerChanged, outerProven, outerNested := c.changedDir, c.dirProven, c.nested
	c.nested = true
	c.statementList(statements, &dir)
	c.changedDir, c.dirProven, c.nested = outerChanged, outerProven, outerNested
}

func (c *classifier) unresolved(reason string) {
	c.add(Classification{
		Effect:     EffectPermanentDeletion,
		Unresolved: reason,
		Reason:     "The command may permanently delete data, and " + reason + ".",
	})
}

// redirect classifies truncating redirects, including combined stdout/stderr.
// Append forms keep the prior bytes.
func (c *classifier) redirect(stmt *syntax.Stmt, dir string) {
	for _, redir := range stmt.Redirs {
		if redir.Op != syntax.RdrOut && redir.Op != syntax.RdrClob && redir.Op != syntax.RdrAll {
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
		if c.directoryUnproven() && !filepath.IsAbs(name) {
			c.unresolved("a directory change makes the redirect destination state unprovable")
			continue
		}
		target, err := c.resolve(dir, name)
		if err == nil && overlappingPaths([]string{target}, c.movePaths) {
			// The file is not there yet, but an earlier move in this command
			// puts its only copy there, and writing would destroy that.
			c.unresolved("an earlier move places data at the redirect destination, so writing there would destroy it")
			continue
		}
		if err != nil || !destroysContent(target) {
			// Creating a file or redirecting to a device keeps stored data.
			continue
		}
		producer := *stmt
		producer.Redirs = nil
		for _, input := range stmt.Redirs {
			if input.Op != syntax.RdrOut && input.Op != syntax.RdrClob && input.Op != syntax.RdrAll {
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
			// Keep positional boundaries and later literal flags. The empty
			// placeholder is never accepted as a proven operand or executable.
			value = ""
		}
		args = append(args, value)
	}
	if len(args) == 0 || args[0] == "" {
		if !complete {
			c.unresolved("the command name is not literal, so its effect cannot be proven")
			*dir = ""
		}
		return
	}
	if args[0] == "cd" || args[0] == "pushd" || args[0] == "popd" {
		// pushd and popd change the directory as well, and where they leave
		// the shell is not modeled.
		if args[0] != "cd" || !complete {
			c.changedDir = true
			c.dirProven = false
			*dir = ""
			return
		}
		*dir = c.changeDirectory(args, *dir)
		return
	}
	c.command(args, complete, *dir)
	if c.directoryOpaque {
		c.directoryOpaque = false
		*dir = ""
	}
}

// changeDirectory returns the directory later statements run in, or an empty
// string when it cannot be proven.
func (c *classifier) changeDirectory(args []string, dir string) string {
	if len(args) != 2 || dir == "" || args[1] == "" {
		c.changedDir = true
		c.dirProven = false
		return ""
	}
	next := args[1]
	absolute := filepath.IsAbs(next)
	if !absolute {
		next = filepath.Join(dir, next)
	}
	next = filepath.Clean(next)
	c.changedDir = true
	// A cd proves the directory only when it names a single literal directory
	// that exists now, in a scope where the following statement is guaranteed
	// to run. A relative operand also needs the current directory proven,
	// because a failed earlier cd would resolve it elsewhere. Otherwise the
	// resolved directory is still used for classification while result()
	// records that it is not provable.
	c.dirProven = !c.dirProofBlocked && (absolute || c.dirProven) && directoryExists(next)
	return next
}

// directoryExists reports a directory that exists at classification time. A
// symbolic link to a directory counts because the shell follows it.
func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func (c *classifier) command(args []string, complete bool, dir string) {
	if len(args) == 0 || args[0] == "" {
		c.unresolved("the command name is not literal, so its effect cannot be proven")
		return
	}
	name := filepath.Base(args[0])
	// `command -v` and `command -V` only resolve a name; they never execute it,
	// so a variable operand cannot dispatch a deletion.
	if name == "command" && commandQueryOnly(args[1:]) {
		return
	}
	if !complete && slices.Contains(args, "") && slices.Contains(
		[]string{"sudo", "doas", "nohup", "time", "env", "nice", "timeout", "setsid", "stdbuf", "xargs", "command", "builtin", "exec", "eval", "sh", "bash", "dash", "zsh", "ksh"}, name) {
		// An expansion can supply options, option values or the executable.
		// Never let its placeholder be consumed as a proven dispatch token.
		c.unresolved("the wrapper contains a nonliteral argument, so command dispatch cannot be proven")
		return
	}
	if !complete && slices.Contains([]string{"cp", "truncate", "dd", "mv", "rsync"}, name) {
		c.unresolved("the command arguments are not literal, so its destructive effects cannot be proven")
		return
	}
	// Wrapper arguments may belong to a nested script rather than the
	// wrapper itself, so a later --help must not hide that script.
	if hasInformationalFlag(args[1:]) && !slices.Contains(
		[]string{"sudo", "doas", "nohup", "time", "env", "nice", "timeout", "setsid", "stdbuf", "xargs", "command", "builtin", "exec", "eval", "sh", "bash", "dash", "zsh", "ksh"}, name) {
		return
	}
	if slices.Contains(removalCommands, name) || slices.Contains([]string{"cp", "truncate", "dd", "find", "git", "mv", "rsync"}, name) {
		start := len(c.effects)
		defer func() {
			for i := start; i < len(c.effects); i++ {
				effect := &c.effects[i]
				if effect.Effect != EffectPermanentDeletion || effect.Operations != nil {
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
		c.wrapper(args, complete, dir)
	case "env", "nice", "timeout", "setsid", "stdbuf":
		c.launchWrapper(args, complete, dir)
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
	case "cd", "pushd", "popd":
		// A directory change hidden behind a builtin wrapper still affects
		// the target identity of later commands, but its result is not proven.
		c.changedDir = true
		c.dirProven = false
		c.directoryOpaque = true
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
		blocked := c.dirProofBlocked
		c.dirProofBlocked = true
		for _, stmt := range script.Stmts {
			c.statement(stmt, &dir)
		}
		c.dirProofBlocked = blocked
		// The evaluated script may have changed the directory, or defined a
		// function that replaces a builtin, and neither is modeled outside
		// the script, so no proof survives it.
		c.dirProven = false
		c.directoryOpaque = true
	case "xargs":
		c.xargs(args, complete, dir)
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
	case "mkdir":
		c.makeDirectories(args, complete, dir)
	case "ln":
		c.makeLinks(args, dir)
	case "mv":
		c.move(args, complete, dir)
	case "cp":
		c.copy(args, dir)
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
		if name == "" {
			continue
		}
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

// move checks the effective destination for every source. Moving into a
// directory can overwrite its existing child just as a direct rename can.
func (c *classifier) move(args []string, complete bool, dir string) {
	if !complete {
		c.unresolved("the move arguments are not literal, so overwritten targets cannot be proven")
		return
	}
	var paths, options []string
	destination := ""
	targetDirectory, noTargetDirectory, noClobber := false, false, false
	flags := true
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if flags && arg == "--" {
			flags = false
			options = append(options, arg)
			continue
		}
		if !flags || !strings.HasPrefix(arg, "-") || arg == "-" {
			paths = append(paths, arg)
			continue
		}
		switch {
		case arg == "-t" || arg == "--target-directory":
			if i+1 >= len(args) {
				c.unresolved("the move target directory is missing")
				return
			}
			i++
			destination = args[i]
			targetDirectory = true
		case strings.HasPrefix(arg, "--target-directory="):
			destination = strings.TrimPrefix(arg, "--target-directory=")
			targetDirectory = true
		case strings.HasPrefix(arg, "-t") && len(arg) > 2:
			destination = arg[2:]
			targetDirectory = true
		case arg == "-T" || arg == "--no-target-directory":
			noTargetDirectory = true
			options = append(options, arg)
		case arg == "--no-clobber":
			noClobber = true
			options = append(options, arg)
		case arg == "--force" || arg == "--interactive":
			noClobber = false
			options = append(options, arg)
		case !strings.HasPrefix(arg, "--"):
			for _, flag := range arg[1:] {
				switch flag {
				case 'n':
					noClobber = true
				case 'f', 'i':
					noClobber = false
				case 'v':
				default:
					c.unresolved("the move option cannot be inspected")
					return
				}
			}
			options = append(options, arg)
		default:
			c.unresolved("the move option cannot be inspected")
			return
		}
	}
	if targetDirectory && noTargetDirectory {
		c.unresolved("the move has conflicting destination options")
		return
	}
	var sources []string
	if targetDirectory {
		sources = paths
	} else {
		if len(paths) < 2 {
			return
		}
		destination = paths[len(paths)-1]
		sources = paths[:len(paths)-1]
	}
	if len(sources) == 0 || destination == "" {
		c.unresolved("the move has no exact source or destination")
		return
	}
	if c.directoryUnproven() {
		for _, path := range append([]string{destination}, sources...) {
			if !filepath.IsAbs(path) {
				c.unresolved("a directory change makes the move source or destination state unprovable")
				return
			}
		}
	}
	target, known := c.destination(dir, destination)
	if !known {
		c.unresolved("the move destination is a symbolic link whose target is not known, so where the data lands cannot be proven; name the real path")
		return
	}
	info, statErr := os.Stat(target)
	if statErr != nil && !os.IsNotExist(statErr) {
		c.unresolved("the move destination cannot be inspected")
		return
	}
	// A directory this command creates is empty when the move runs, so nothing
	// stored before the command can be under it.
	created := os.IsNotExist(statErr) && slices.Contains(c.createdDirectories, target)
	intoDirectory := ((statErr == nil && info.IsDir()) || created) && !noTargetDirectory
	if (targetDirectory && !intoDirectory) || (len(sources) > 1 && !intoDirectory) {
		c.unresolved("the move needs a verified destination directory")
		return
	}
	var overwritten, moved, affected []string
	operations := make(map[string]string)
	seen := make(map[string]bool)
	for _, source := range sources {
		sourcePath, err := c.resolve(dir, source)
		if err != nil {
			c.unresolved("a move source cannot be resolved")
			return
		}
		moved = append(moved, sourcePath)
		effective := target
		if intoDirectory {
			effective = filepath.Join(target, filepath.Base(filepath.Clean(source)))
		}
		paths := []string{sourcePath, effective}
		if overlappingPaths(paths, c.movePaths) || overlappingPaths(paths, c.destructivePaths) {
			c.unresolved("an earlier operation changes a move source or destination, so overwritten data cannot be identified safely")
			return
		}
		affected = append(affected, paths...)
		_, err = os.Lstat(effective)
		if err != nil && !os.IsNotExist(err) {
			c.unresolved("a move destination cannot be inspected")
			return
		}
		if !noClobber && seen[effective] {
			c.unresolved("multiple move sources address the same destination, so overwritten data cannot be identified safely")
			return
		}
		if !noClobber && err == nil {
			overwritten = append(overwritten, effective)
			descriptor := append([]string{args[0]}, options...)
			descriptor = append(descriptor, sourcePath, effective)
			data, _ := json.Marshal([][]string{descriptor})
			operations = mergeOperations(operations, map[string]string{effective: string(data)})
		}
		seen[effective] = true
	}
	if len(overwritten) > 0 {
		c.add(Classification{Effect: EffectPermanentDeletion, Command: "mv", Targets: overwritten, Operations: operations, Reason: "The move replaces existing destination data."})
		c.movePaths = append(c.movePaths, affected...)
		return
	}
	c.add(Classification{Effect: EffectQuarantine, Command: "mv", Targets: moved, Reason: "The command moves data to " + target + "; a later move can restore it."})
	c.movePaths = append(c.movePaths, affected...)
}

// copy checks effective destinations rather than treating a copy as harmless:
// overwriting an existing file destroys its prior contents just like a redirect.
func (c *classifier) copy(args []string, dir string) {
	var paths []string
	var retainedOptions []string
	var destination string
	options, noClobber, noTargetDirectory, targetDirectory := true, false, false, false
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if options && strings.HasPrefix(arg, "-") && arg != "-" {
			retainedOptions = append(retainedOptions, arg)
		}
		switch {
		case options && arg == "--":
			options = false
		case !options || !strings.HasPrefix(arg, "-") || arg == "-":
			paths = append(paths, arg)
		case arg == "-t" || arg == "--target-directory":
			if i+1 >= len(args) {
				c.unresolved("the copy destination is missing")
				return
			}
			i++
			destination, targetDirectory = args[i], true
			retainedOptions = append(retainedOptions, args[i])
		case strings.HasPrefix(arg, "--target-directory="):
			destination, targetDirectory = strings.TrimPrefix(arg, "--target-directory="), true
		case strings.HasPrefix(arg, "-t") && len(arg) > 2:
			destination, targetDirectory = arg[2:], true
		case arg == "-T" || arg == "--no-target-directory":
			noTargetDirectory = true
		case arg == "--no-clobber":
			noClobber = true
		case arg == "--force" || arg == "--interactive":
			noClobber = false
		case !strings.HasPrefix(arg, "--"):
			for _, flag := range arg[1:] {
				switch flag {
				case 'n':
					noClobber = true
				case 'f', 'i':
					noClobber = false
				case 'R', 'r', 'a', 'p', 'v', 'P', 'L', 'H':
				default:
					c.unresolved("the copy option cannot be inspected")
					return
				}
			}
		default:
			c.unresolved("the copy option cannot be inspected")
			return
		}
	}
	if noClobber {
		return
	}
	if !targetDirectory {
		if len(paths) < 2 {
			return
		}
		destination, paths = paths[len(paths)-1], paths[:len(paths)-1]
	}
	if destination == "" || len(paths) == 0 || targetDirectory && noTargetDirectory {
		c.unresolved("the copy has no verified source and destination")
		return
	}
	target, known := c.destination(dir, destination)
	if !known {
		c.unresolved("the copy destination is a symbolic link whose target is not known, so where the data lands cannot be proven; name the real path")
		return
	}
	if c.directoryUnproven() {
		c.unresolved("the copy destination cannot be resolved safely")
		return
	}
	info, err := os.Stat(target)
	if err != nil && !os.IsNotExist(err) {
		c.unresolved("the copy destination cannot be inspected")
		return
	}
	created := os.IsNotExist(err) && slices.Contains(c.createdDirectories, target)
	intoDirectory := ((err == nil && info.IsDir()) || created) && !noTargetDirectory
	if !intoDirectory && (targetDirectory || len(paths) > 1) {
		c.unresolved("the copy needs a verified destination directory")
		return
	}
	var changed []string
	for _, source := range paths {
		effective := target
		if intoDirectory {
			effective = filepath.Join(target, filepath.Base(filepath.Clean(source)))
		}
		if overlappingPaths([]string{effective}, c.movePaths) || overlappingPaths([]string{effective}, changed) {
			c.unresolved("an earlier write changes the copy destination, so its original data cannot be identified safely")
			return
		}
		changed = append(changed, effective)
		if existing, err := os.Lstat(effective); os.IsNotExist(err) {
			continue
		} else if err != nil {
			c.unresolved("the effective copy destination cannot be inspected")
			return
		} else if existing.IsDir() {
			// Recursive merges may follow existing nested links. A directory
			// snapshot alone does not approve the referents those writes reach.
			c.unresolved("a copy merges an existing directory whose exact overwritten entries cannot be proven")
			return
		}
		sourcePath, err := c.resolve(dir, source)
		if err != nil {
			c.unresolved("the copy source cannot be resolved safely")
			return
		}
		descriptor := append([]string{args[0]}, retainedOptions...)
		descriptor = append(descriptor, sourcePath, effective)
		data, _ := json.Marshal([][]string{descriptor})
		c.add(Classification{
			Effect:     EffectPermanentDeletion,
			Command:    "cp",
			Targets:    []string{effective},
			Operations: map[string]string{effective: string(data)},
			Reason:     "The copy replaces existing destination data.",
		})
	}
	c.movePaths = append(c.movePaths, changed...)
}

// destination resolves the path a move or copy writes to. A destination that
// is a symbolic link stands for somewhere else, so it is followed only when
// this command created it from a literal target. Any other link leaves the
// real destination unknown. The recorded links are read before the
// filesystem, because a link this command creates does not exist yet.
func (c *classifier) destination(dir, destination string) (string, bool) {
	target, err := c.resolve(dir, destination)
	if err != nil {
		return "", false
	}
	for range 8 {
		created, ok := c.createdLinks[target]
		if !ok {
			info, err := os.Lstat(target)
			if errors.Is(err, os.ErrNotExist) && c.unknownLinks {
				// It may be a link this command creates under a name that
				// could not be read.
				return "", false
			}
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				return target, true
			}
			return "", false
		}
		if created == "" {
			// This command creates the link, but its target is not literal.
			return "", false
		}
		if !filepath.IsAbs(created) {
			created = filepath.Join(filepath.Dir(target), created)
		}
		target = filepath.Clean(created)
	}
	return "", false
}

// makeLinks remembers each symbolic link this ln creates, so a later move or
// copy can resolve a destination through it. Only the command's own scope is
// read, and only the one- and two-operand forms of ln -s with every word
// literal: an option that changes which operand is the link would put the
// link somewhere else. Any other ln that may create a link sets unknownLinks,
// which makes a destination that does not exist yet suspect.
func (c *classifier) makeLinks(args []string, dir string) {
	if c.dirProofBlocked || c.nested || c.asynchronous {
		return
	}
	symbolic, options, unread := false, true, false
	var operands []string
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if options && arg == "--" {
			options = false
			continue
		}
		if options && len(operands) == 0 && (arg == "" || arg != "-" && strings.HasPrefix(arg, "-")) {
			switch {
			case arg == "--symbolic":
				symbolic = true
			case arg != "" && strings.Trim(arg[1:], "sfinvh") == "":
				symbolic = symbolic || strings.Contains(arg, "s")
			default:
				unread = true
			}
			continue
		}
		operands = append(operands, arg)
	}
	if unread {
		// A lone plain literal operand names a link either way: as the link
		// of a two-operand form, or as the target of a one-operand form,
		// whose link takes the target's name in the current directory. Where
		// it points is not known. Anything else could put a link anywhere.
		if len(operands) != 1 || operands[0] == "" || operands[0] == "." || strings.Contains(operands[0], "/") {
			c.unknownLinks = true
			return
		}
		operands = []string{"", operands[0]}
	}
	if !symbolic || len(operands) < 1 || len(operands) > 2 {
		if symbolic {
			c.unknownLinks = true
		}
		return
	}
	target, link := operands[0], operands[0]
	if len(operands) == 2 {
		link = operands[1]
	}
	if link == "" || link == "." {
		c.unknownLinks = true
		return
	}
	if c.directoryUnproven() && !filepath.IsAbs(link) {
		return
	}
	path, err := c.resolve(dir, link)
	if err != nil {
		return
	}
	if c.createdLinks == nil {
		c.createdLinks = make(map[string]string)
	}
	c.createdLinks[path] = target
}

// makeDirectories remembers each directory this mkdir creates, so a later
// statement can move or copy into it. Only a literal mkdir in the command's
// own scope, in a statement that finishes before the next one starts, can
// vouch for a directory: one inside a subshell, a branch or a nested script
// cannot.
func (c *classifier) makeDirectories(args []string, complete bool, dir string) {
	if !complete || c.dirProofBlocked || c.nested || c.asynchronous {
		return
	}
	parents := false
	var paths []string
	options := true
	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case options && arg == "--":
			options = false
		case options && arg == "--parents":
			parents = true
		case options && arg == "--verbose":
		case options && (arg == "-m" || arg == "--mode"):
			i++
		case options && strings.HasPrefix(arg, "--mode="):
		case options && strings.HasPrefix(arg, "-") && arg != "-":
			flags, mode, hasMode := strings.Cut(arg[1:], "m")
			if strings.Trim(flags, "pv") != "" {
				return
			}
			if hasMode && mode == "" {
				i++
			}
			parents = parents || strings.Contains(flags, "p")
		default:
			paths = append(paths, arg)
		}
	}
	for _, path := range paths {
		if path == "" || (c.directoryUnproven() && !filepath.IsAbs(path)) {
			continue
		}
		target, err := c.resolve(dir, path)
		if err != nil {
			continue
		}
		for {
			_, err := os.Lstat(target)
			if err == nil || !errors.Is(err, os.ErrNotExist) {
				break
			}
			c.createdDirectories = append(c.createdDirectories, target)
			if !parents {
				break
			}
			parent := filepath.Dir(target)
			if parent == target {
				break
			}
			target = parent
		}
	}
}

// wrapper consumes only documented options whose effect is understood. Unknown
// or directory-changing options cannot hide an uninspected command.
func (c *classifier) wrapper(args []string, complete bool, dir string) {
	name := filepath.Base(args[0])
	i := 1
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			i++
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			if name == "sudo" && strings.Contains(arg, "=") {
				c.unresolved("wrapper environment assignments can change command resolution")
				return
			}
			break
		}
		if (arg == "--help" || arg == "--version") && i == 1 && len(args) == 2 {
			return
		}
		switch name {
		case "sudo":
			if slices.Contains([]string{"--non-interactive", "--preserve-env", "--set-home", "--stdin", "--reset-timestamp"}, arg) {
				i++
				continue
			}
			if slices.Contains([]string{"--user", "--group", "--prompt", "--close-from", "--command-timeout", "--role", "--type"}, arg) {
				if i+1 >= len(args) {
					c.unresolved("a wrapper option value is missing")
					return
				}
				i += 2
				continue
			}
			if strings.HasPrefix(arg, "--") {
				key, _, hasValue := strings.Cut(arg, "=")
				if hasValue && slices.Contains([]string{"--user", "--group", "--prompt", "--close-from", "--command-timeout", "--role", "--type", "--preserve-env"}, key) {
					i++
					continue
				}
				c.unresolved("the wrapper option cannot be inspected")
				return
			}
		case "doas":
			// Short flags below include the documented user/style values.
		case "nohup":
			c.unresolved("the wrapper option cannot be inspected")
			return
		case "time":
			if arg == "--" {
				i++
				continue
			}
			if slices.Contains([]string{"--portability", "--verbose", "--quiet"}, arg) {
				i++
				continue
			}
			if arg == "--format" {
				if i+1 >= len(args) {
					c.unresolved("a wrapper option value is missing")
					return
				}
				i += 2
				continue
			}
		}
		accepted := true
		for j := 1; j < len(arg); j++ {
			flag := arg[j]
			value := false
			switch name {
			case "sudo":
				if strings.ContainsRune("nEHSk", rune(flag)) {
					continue
				}
				value = strings.ContainsRune("ugpCTrt", rune(flag))
			case "doas":
				if strings.ContainsRune("nL", rune(flag)) {
					continue
				}
				value = strings.ContainsRune("ua", rune(flag))
			case "time":
				if strings.ContainsRune("pvqa", rune(flag)) {
					continue
				}
				value = flag == 'f'
			}
			if !value {
				accepted = false
				break
			}
			if j+1 == len(arg) {
				if i+1 >= len(args) {
					accepted = false
					break
				}
				i++
			}
			break
		}
		if !accepted {
			c.unresolved("the wrapper option cannot be inspected")
			return
		}
		i++
	}
	if i >= len(args) {
		if !complete {
			c.unresolved("the wrapped command is not literal")
		}
		return
	}
	if name == "sudo" && strings.Contains(args[i], "=") {
		c.unresolved("wrapper environment assignments can change command resolution")
		return
	}
	c.command(args[i:], complete, dir)
}

// launchWrapper inspects common process launchers without allowing their
// option arguments or timeout duration to masquerade as the executable.
func (c *classifier) launchWrapper(args []string, complete bool, dir string) {
	name := filepath.Base(args[0])
	i := 1
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			i++
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			break
		}
		if (arg == "--help" || arg == "--version") && i == 1 && len(args) == 2 {
			return
		}
		consumed := false
		switch name {
		case "env":
			// Even an otherwise harmless executable may change with PATH, loader,
			// split-string, chdir, or other environment-dispatch options.
		case "nice":
			if arg == "-n" || arg == "--adjustment" {
				if i+1 < len(args) && decimalInteger(args[i+1]) {
					i++
					consumed = true
				}
			} else if value, ok := strings.CutPrefix(arg, "--adjustment="); ok {
				consumed = decimalInteger(value)
			} else if strings.HasPrefix(arg, "-n") && len(arg) > 2 {
				consumed = decimalInteger(arg[2:])
			} else {
				consumed = decimalInteger(arg)
			}
		case "timeout":
			if slices.Contains([]string{"--preserve-status", "--foreground", "--verbose", "-v"}, arg) {
				consumed = true
			} else if slices.Contains([]string{"-s", "--signal", "-k", "--kill-after"}, arg) {
				if i+1 < len(args) {
					i++
					consumed = true
				}
			} else if strings.HasPrefix(arg, "--signal=") || strings.HasPrefix(arg, "--kill-after=") ||
				(len(arg) > 2 && (strings.HasPrefix(arg, "-s") || strings.HasPrefix(arg, "-k"))) {
				consumed = true
			}
		case "setsid":
			if slices.Contains([]string{"--ctty", "--fork", "--wait"}, arg) {
				consumed = true
			} else if !strings.HasPrefix(arg, "--") {
				consumed = len(arg) > 1
				for _, flag := range arg[1:] {
					if !strings.ContainsRune("cfw", flag) {
						consumed = false
					}
				}
			}
		case "stdbuf":
			if slices.Contains([]string{"-i", "-o", "-e", "--input", "--output", "--error"}, arg) {
				if i+1 < len(args) {
					i++
					consumed = true
				}
			} else if strings.HasPrefix(arg, "--input=") || strings.HasPrefix(arg, "--output=") || strings.HasPrefix(arg, "--error=") ||
				(len(arg) > 2 && (strings.HasPrefix(arg, "-i") || strings.HasPrefix(arg, "-o") || strings.HasPrefix(arg, "-e"))) {
				consumed = true
			}
		}
		if !consumed {
			c.unresolved("the process-launcher option or value cannot be inspected")
			return
		}
		i++
	}
	if name == "env" && i < len(args) && (args[i] == "-" || strings.Contains(args[i], "=")) {
		c.unresolved("environment changes can alter command resolution or inject execution")
		return
	}
	if name == "timeout" {
		if i >= len(args) || !timeoutDuration(args[i]) {
			c.unresolved("the timeout duration cannot be inspected")
			return
		}
		i++
	}
	if i >= len(args) {
		if !complete {
			c.unresolved("the launched command is not literal")
		}
		return
	}
	c.command(args[i:], complete, dir)
}

func decimalInteger(value string) bool {
	if len(value) > 0 && (value[0] == '+' || value[0] == '-') {
		value = value[1:]
	}
	if value == "" {
		return false
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func timeoutDuration(value string) bool {
	value = strings.TrimPrefix(value, "+")
	if len(value) > 0 && strings.ContainsRune("smhd", rune(value[len(value)-1])) {
		value = value[:len(value)-1]
	}
	digits, dots := 0, 0
	for _, character := range value {
		if character == '.' {
			dots++
		} else if character >= '0' && character <= '9' {
			digits++
		} else {
			return false
		}
	}
	return digits > 0 && dots <= 1
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
		if !complete {
			c.unresolved("nonliteral find arguments may introduce an uninspected destructive action")
		}
		return
	}
	var roots []string
	for _, arg := range args[1:] {
		if arg == "" {
			continue
		}
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

// git classifies the git subcommands that destroy working-tree data. Read-only
// subcommands (diff, log, show, status, stash show, branch listing) never need
// the deletion gate, so a nonliteral ref such as @{0} or HEAD~1 is harmless
// there. Destructive subcommands still fail closed when their operands cannot
// be proven.
func (c *classifier) git(args []string, complete bool, dir string) {
	if len(args) < 2 {
		return
	}
	subcommand, ok := gitSubcommand(args)
	if !ok {
		c.unresolved("git global options can change the repository or command dispatch")
		return
	}
	if subcommand == len(args) {
		return
	}
	if args[subcommand] == "" {
		c.unresolved("the git subcommand is not literal, so its effect cannot be proven")
		return
	}
	if subcommand > 1 {
		// Behind options only a built-in subcommand can be read by name: an
		// option can define an alias that replaces any other. git rm is left
		// out because which repository the options select decides which files
		// it unlinks.
		if args[subcommand] == "rm" || !slices.Contains(gitBuiltins, args[subcommand]) {
			c.unresolved("git global options can change the repository or command dispatch")
			return
		}
		args = append([]string{"git"}, args[subcommand:]...)
	}
	switch args[1] {
	case "restore":
		if restoresWorktree(args[1:]) {
			c.unresolved("git restore can discard working-tree data whose exact affected set cannot be proven")
		}
	case "checkout":
		c.checkout(args[2:], complete)
	case "switch":
		if slices.Contains(args[1:], "-f") || slices.Contains(args[1:], "--force") || slices.Contains(args[1:], "--discard-changes") {
			c.unresolved("git switch --force can discard working-tree changes whose exact affected set cannot be proven")
		}
	case "branch":
		for _, arg := range args[2:] {
			if arg == "-d" || arg == "-D" || arg == "--delete" {
				c.add(Classification{
					Effect:     EffectPermanentDeletion,
					Command:    "git branch --delete",
					Unresolved: "git branch deletion removes a ref whose commits cannot be listed before it runs",
					Reason:     "The command git branch --delete permanently removes a branch reference, and the exact affected commits cannot be proven.",
				})
				return
			}
		}
	case "stash":
		if len(args) < 3 {
			return
		}
		if args[2] == "drop" || args[2] == "clear" {
			c.add(Classification{
				Effect:     EffectPermanentDeletion,
				Command:    "git stash " + args[2],
				Unresolved: "git stash " + args[2] + " removes stash entries whose contents cannot be listed safely",
				Reason:     "The command git stash " + args[2] + " permanently deletes stash data, and the exact affected entries cannot be proven.",
			})
		}
	case "rm":
		_, paths := operands(args[1:])
		c.removal("git rm", paths, false, complete, dir)
	case "clean":
		if hasDryRunFlag(args[1:]) && complete {
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
		if !complete {
			c.unresolved("git reset arguments are not literal, so a --hard form cannot be excluded")
			return
		}
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

// checkout reports whether a git checkout form writes the working tree or the
// branch ref. Creating a branch (-b) is safe; path restores and forced
// checkouts are not. A plain branch switch refuses to overwrite local changes,
// so it is not classified as permanent deletion.
func (c *classifier) checkout(args []string, complete bool) {
	operandCount := 0
	for _, arg := range args {
		switch {
		case arg == "-b" || arg == "--branch":
			return
		case arg == "-B" || arg == "--force" || arg == "-f" || arg == "--patch" || arg == "-p" || arg == "--ours" || arg == "--theirs" || arg == "--":
			c.unresolved("git checkout can discard working-tree data whose exact affected set cannot be proven")
			return
		case strings.HasPrefix(arg, "-") && arg != "-":
			continue
		default:
			operandCount++
			if arg == "." || strings.HasPrefix(arg, "./") || filepath.IsAbs(arg) {
				c.unresolved("git checkout can discard working-tree data whose exact affected set cannot be proven")
				return
			}
		}
	}
	// Two operands select a tree and a path, which restores that path.
	if operandCount >= 2 {
		c.unresolved("git checkout can discard working-tree data whose exact affected set cannot be proven")
	}
}

// restoresWorktree reports whether git restore writes the working tree. A
// --staged-only restore changes the index without touching stored files.
func restoresWorktree(args []string) bool {
	staged := false
	worktree := false
	options := true
	for _, arg := range args[1:] {
		if !options {
			continue
		}
		switch arg {
		case "--":
			options = false
		case "--staged", "-S":
			staged = true
		case "--worktree", "-W":
			worktree = true
		case "--help", "-h", "--version":
			return false
		}
	}
	return worktree || !staged
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

// xargs delegates its explicit child to the same command classifier. Input
// arguments are generated, so destructive child effects cannot prove targets.
func (c *classifier) xargs(args []string, complete bool, dir string) {
	i := 1
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			i++
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			break
		}
		if (arg == "--help" || arg == "--version") && i == 1 && len(args) == 2 {
			return
		}
		if slices.Contains([]string{"-0", "--null", "-r", "--no-run-if-empty", "-t", "--verbose", "-p", "--interactive", "-x", "--exit"}, arg) {
			i++
			continue
		}
		// GNU optional values belong to this token (long =value or short
		// attached value). A bare optional flag must leave the child intact.
		if slices.Contains([]string{"--eof", "--replace", "--max-lines"}, arg) ||
			(len(arg) >= 2 && !strings.HasPrefix(arg, "--") && strings.ContainsRune("eil", rune(arg[1]))) {
			i++
			continue
		}
		if slices.Contains([]string{"-a", "--arg-file", "-d", "--delimiter", "-E", "-I", "-L", "-n", "--max-args", "-P", "--max-procs", "-s", "--max-chars"}, arg) {
			if i+1 >= len(args) {
				c.unresolved("an xargs option value is missing")
				return
			}
			i += 2
			continue
		}
		key, _, hasValue := strings.Cut(arg, "=")
		if hasValue && slices.Contains([]string{"--arg-file", "--delimiter", "--eof", "--replace", "--max-lines", "--max-args", "--max-procs", "--max-chars"}, key) {
			i++
			continue
		}
		if len(arg) > 2 && !strings.HasPrefix(arg, "--") && strings.ContainsRune("adEILnPs", rune(arg[1])) {
			i++
			continue
		}
		c.unresolved("the xargs option cannot be inspected")
		return
	}
	if i >= len(args) {
		if !complete {
			c.unresolved("the xargs command is not literal")
		}
		return
	}
	start := len(c.effects)
	c.command(args[i:], false, dir)
	for index := start; index < len(c.effects); index++ {
		effect := &c.effects[index]
		if effect.IsDestructive() {
			effect.Unresolved = "xargs builds its target arguments from input, so the exact targets cannot be proven"
			effect.Reason = "The launched command may permanently delete data, and " + effect.Unresolved + "."
		}
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
		script, err := syntax.NewParser().Parse(strings.NewReader(args[i+1]), "")
		if err != nil {
			c.unresolved("the nested shell script cannot be parsed")
			return
		}
		c.localDirectoryStatements(script.Stmts, nestedDir)
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

// gitFlagOptions are the git options before the subcommand that take no value.
// --bare does choose a repository (the current directory, read as a bare one),
// which is why a gated subcommand behind options is still analyzed and git rm
// behind them stays unresolved.
var gitFlagOptions = []string{
	"-p", "--paginate", "-P", "--no-pager", "--bare",
	"--literal-pathspecs", "--glob-pathspecs", "--noglob-pathspecs", "--icase-pathspecs",
	"--no-replace-objects", "--no-optional-locks", "--no-advice",
}

// gitValueOptionPrefixes are git long options whose value is attached after "=".
var gitValueOptionPrefixes = []string{"--git-dir=", "--namespace=", "--super-prefix=", "--work-tree="}

// gitBuiltins are the git subcommands compiled into git itself. Git runs a
// built-in before it looks for an alias or a git-* program, so no option can
// replace one. Only names that have been built in for years are listed: on a
// git that lacks one it could be an alias.
var gitBuiltins = []string{
	"add", "am", "annotate", "apply", "archive", "bisect", "blame", "branch", "bundle",
	"cat-file", "check-attr", "check-ignore", "check-mailmap", "check-ref-format",
	"checkout", "checkout-index", "cherry", "cherry-pick", "clean", "clone", "column",
	"commit", "commit-tree", "config", "count-objects", "describe", "diff", "diff-files",
	"diff-index", "diff-tree", "difftool", "fast-export", "fast-import", "fetch", "fetch-pack",
	"fmt-merge-msg", "for-each-ref", "format-patch", "fsck", "fsck-objects", "gc",
	"get-tar-commit-id", "grep", "hash-object", "help", "index-pack", "init", "init-db",
	"interpret-trailers", "log", "ls-files", "ls-remote", "ls-tree", "mailinfo", "mailsplit",
	"merge", "merge-base", "merge-file", "merge-index", "merge-ours", "merge-recursive",
	"merge-subtree", "merge-tree", "mergetool", "mktag", "mktree", "mv", "name-rev", "notes",
	"pack-objects", "pack-redundant", "pack-refs", "patch-id", "prune", "prune-packed", "pull",
	"push", "read-tree", "receive-pack", "reflog", "remote", "remote-ext", "remote-fd",
	"repack", "replace", "rerere", "reset", "restore", "rev-list", "rev-parse", "revert", "rm",
	"send-pack", "shortlog", "show", "show-branch", "show-index", "show-ref", "stash", "status",
	"stripspace", "submodule", "switch", "symbolic-ref", "tag", "unpack-file", "unpack-objects",
	"update-index", "update-ref", "update-server-info", "upload-archive", "upload-pack", "var",
	"verify-commit", "verify-pack", "verify-tag", "version", "whatchanged", "worktree", "write-tree",
}

// gitConfigDispatches reports a -c or --config-env setting that can run another
// command or pull in more configuration: an alias, an include, or a hook path.
// A setting with no readable name cannot be checked, so it counts.
func gitConfigDispatches(setting string) bool {
	name, _, _ := strings.Cut(setting, "=")
	// Git ignores case and surrounding whitespace in a key.
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "" || strings.HasPrefix(name, "alias.") || strings.HasPrefix(name, "include.") ||
		strings.HasPrefix(name, "includeif.") || name == "core.hookspath" || name == "core.fsmonitor"
}

// gitSubcommand returns the index of the subcommand after git's leading
// options and whether those options may be stepped over. It reports len(args)
// when an informational option ends the command.
func gitSubcommand(args []string) (int, bool) {
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "" || !strings.HasPrefix(arg, "-") {
			return i, true
		}
		switch {
		case arg == "-h" || arg == "--help" || arg == "-v" || arg == "--version" ||
			arg == "--html-path" || arg == "--man-path" || arg == "--info-path":
			return len(args), true
		case arg == "-C" || arg == "--git-dir" || arg == "--work-tree" || arg == "--namespace" || arg == "--super-prefix":
			// The next argument is the value, literal or not.
			i++
		case arg == "-c" || arg == "--config-env":
			if i+1 >= len(args) || gitConfigDispatches(args[i+1]) {
				return i, false
			}
			i++
		case strings.HasPrefix(arg, "--config-env="):
			if gitConfigDispatches(strings.TrimPrefix(arg, "--config-env=")) {
				return i, false
			}
		case slices.Contains(gitFlagOptions, arg) || slices.ContainsFunc(gitValueOptionPrefixes, func(prefix string) bool { return strings.HasPrefix(arg, prefix) }):
		default:
			return i, false
		}
	}
	return len(args), true
}

// hasInformationalFlag recognizes only an unambiguous help/version invocation.
// In a longer argument list the token may be an operand after --, a find
// predicate value, or another option's value rather than an informational flag.
func hasInformationalFlag(args []string) bool {
	return len(args) == 1 && (args[0] == "--help" || args[0] == "--version")
}

// commandQueryOnly reports the `command -v`/`command -V` query form. It
// resolves a command name without executing the resolved command, so operands
// that cannot be proven are not a dispatch risk.
func commandQueryOnly(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && len(arg) > 1 {
			if strings.ContainsAny(arg[1:], "vV") {
				return true
			}
			continue
		}
		return false
	}
	return false
}

// hasDryRunFlag reports a documented dry run for git clean. Short flags may be
// clustered, as in "-nd".
func hasDryRunFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
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
			// A bare "[" never opens a character class and is the POSIX test
			// builtin in command position. Every other word containing "["
			// may be a glob and stays unproven.
			if strings.Contains(value.Value, `\`) || strings.ContainsAny(value.Value, "*?~{") ||
				(strings.Contains(value.Value, "[") && value.Value != "[") {
				return "", false
			}
			text.WriteString(value.Value)
		case *syntax.SglQuoted:
			text.WriteString(value.Value)
		case *syntax.DblQuoted:
			inner, ok := literalQuotedWord(&syntax.Word{Parts: value.Parts})
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

// literalQuotedWord returns a double-quoted word's text. Globs, braces and
// tilde characters are literal inside double quotes, while parameter, command
// and arithmetic expansions are not.
func literalQuotedWord(word *syntax.Word) (string, bool) {
	var text strings.Builder
	for _, part := range word.Parts {
		switch value := part.(type) {
		case *syntax.Lit:
			if strings.Contains(value.Value, `\`) {
				return "", false
			}
			text.WriteString(value.Value)
		case *syntax.SglQuoted:
			text.WriteString(value.Value)
		case *syntax.DblQuoted:
			inner, ok := literalQuotedWord(&syntax.Word{Parts: value.Parts})
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
