package exec

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/murkl/oak/internal/i18n"
)

// controllingTerminal is the terminal this process was started on, whatever
// its three channels were pointed at.
const controllingTerminal = "/dev/tty"

// Handover is a script given the terminal outright on all three channels for as
// long as it runs, never what this process inherited, which may be the journal.
// It gets a foreground process group of its own, and this process takes the
// terminal back afterwards: an interactive shell hands back a group that is
// gone by then.
type Handover struct {
	cmd *exec.Cmd
	tty string
}

func (h *Handover) SetStdin(io.Reader)  {}
func (h *Handover) SetStdout(io.Writer) {}
func (h *Handover) SetStderr(io.Writer) {}

// Run gives the script the terminal, waits for it, and takes the terminal back.
func (h *Handover) Run() error {
	tty, err := os.OpenFile(h.tty, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("There is no terminal to hand over."), err)
	}
	defer tty.Close()
	h.cmd.Stdin, h.cmd.Stdout, h.cmd.Stderr = tty, tty, tty

	// Only a terminal this process controls has a foreground to hand over. Any
	// other is simply written to and read from.
	fd := int(tty.Fd())
	own, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP)
	if err != nil {
		return h.cmd.Run()
	}
	h.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Foreground: true, Ctty: fd}
	ran := h.cmd.Run()
	return errors.Join(ran, reclaim(fd, own))
}

// reclaim makes a group the terminal's foreground again. Asked from what is
// then a background group, which the kernel stops for it unless the stop is
// ignored for the length of the call.
func reclaim(fd, pgrp int) error {
	signal.Ignore(syscall.SIGTTOU)
	defer signal.Reset(syscall.SIGTTOU)
	return unix.IoctlSetPointerInt(fd, unix.TIOCSPGRP, pgrp)
}
