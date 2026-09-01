package sr

import (
	"os/exec"
	"syscall"
)

// ExecProcess replaces the current process with argv, running under env.
//
// Replacing rather than spawning gets sr out of the way once the credentials
// are in place: the terminal, the signals and the exit status all belong to
// the command itself, with no parent left holding the credentials beside it.
// It is also why sr is Unix-only.
func ExecProcess(argv, env []string) error {
	path, err := exec.LookPath(argv[0])

	if err != nil {
		return err
	}

	return syscall.Exec(path, argv, env)
}
