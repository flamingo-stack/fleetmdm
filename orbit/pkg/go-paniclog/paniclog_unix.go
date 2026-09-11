// Log the panic under unix to the log file

//go:build !windows && !solaris && !plan9
// +build !windows,!solaris,!plan9

package paniclog

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func redirectStderr(f *os.File) (UndoFunction, error) {
	stderrFd := int(os.Stderr.Fd())
	oldfd, err := unix.Dup(stderrFd)
	if err != nil {
		return nil, fmt.Errorf("redirect stderr to file: %w", err)
	}

	err = unix.Dup2(int(f.Fd()), stderrFd)
	if err != nil {
		return nil, fmt.Errorf("redirect stderr to file: %w", err)
	}

	undo := func() error {
		undoErr := unix.Dup2(oldfd, stderrFd)
		unix.Close(oldfd)

		if undoErr != nil {
			return fmt.Errorf("reverse stderr redirection: %w", undoErr)
		}

		return nil
	}

	return undo, nil
}
