package shell

import (
	"fmt"

	"mvdan.cc/sh/v3/syntax"
)

// supportedRedirs is the set of redirect operators that the mvdan.cc/sh
// interpreter can actually execute. It mirrors the switch in
// interp.Runner.redir, which handles exactly these operators and nothing else.
//
// Anything outside this set (<> , >| , >>| , &>| , &>>| ) is rejected before
// execution: mvdan.cc/sh/v3 <= v3.12.1 panics on them with "unhandled redirect
// op", and a panic that unwinds out of a background shell goroutine aborts the
// whole process. Newer releases return an error instead, but a panic-free
// pre-flight check keeps the failure a normal shell error on every version.
var supportedRedirs = map[syntax.RedirOperator]bool{
	syntax.RdrIn:    true,
	syntax.RdrOut:   true,
	syntax.AppOut:   true,
	syntax.DplIn:    true,
	syntax.DplOut:   true,
	syntax.RdrAll:   true,
	syntax.AppAll:   true,
	syntax.Hdoc:     true,
	syntax.DashHdoc: true,
	syntax.WordHdoc: true,
}

// validateRedirs walks a parsed command and returns an error for the first
// redirect operator the interpreter cannot execute. A nil file is a no-op.
func validateRedirs(file *syntax.File) error {
	if file == nil {
		return nil
	}

	var err error
	syntax.Walk(file, func(node syntax.Node) bool {
		if err != nil {
			return false
		}

		stmt, ok := node.(*syntax.Stmt)
		if !ok {
			return true
		}

		for _, rd := range stmt.Redirs {
			if rd == nil || supportedRedirs[rd.Op] {
				continue
			}
			err = fmt.Errorf("unsupported redirect operator %s: the shell interpreter cannot execute it; use a plain input or output redirect", rd.Op)
			return false
		}

		return true
	})

	return err
}
