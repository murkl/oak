package exec

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// terminal opens a pty pair and answers with both ends: the one a program
// writes to, by path, and the one this test reads what it wrote from.
func terminal(t *testing.T) (string, *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pty on this machine: %v", err)
	}
	t.Cleanup(func() { master.Close() })
	fd := int(master.Fd())
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("/dev/pts/%d", n), master
}

// The terminal is handed over outright: what the script prints on either
// channel reaches it, whatever this process's own were pointed at.
func TestAHandoverWritesToTheTerminalItWasGiven(t *testing.T) {
	path, master := terminal(t)
	h := sh.Terminal(Script{Shell: "echo out; echo err >&2"}, Env(os.Environ()))
	h.tty = path

	if err := h.Run(); err != nil {
		t.Fatal(err)
	}

	lines := bufio.NewScanner(master)
	var got []string
	for len(got) < 2 && lines.Scan() {
		got = append(got, strings.TrimSpace(lines.Text()))
	}
	if strings.Join(got, " ") != "out err" {
		t.Errorf("terminal read %q, want both channels", got)
	}
}

// A script that says no is a failure with its own status, as any other is.
func TestAHandoverAnswersWithTheScriptsStatus(t *testing.T) {
	path, _ := terminal(t)
	h := sh.Terminal(Script{Shell: "exit 3"}, Env(os.Environ()))
	h.tty = path

	f, ok := sh.Fail(Step{Name: "Shell"}, h.Run()).(*Failure)

	if !ok || f.Code != 3 {
		t.Errorf("failure = %v, want exit code 3", f)
	}
}

// Nothing to hand over is said, rather than a script started with nobody able
// to see it.
func TestAHandoverWithoutATerminalSaysSo(t *testing.T) {
	h := sh.Terminal(Script{Shell: "true"}, Env(os.Environ()))
	h.tty = "/definitely/not/a/terminal"

	f, ok := sh.Fail(Step{Name: "Shell"}, h.Run()).(*Failure)

	if !ok || !strings.Contains(f.Stderr, "no terminal") {
		t.Errorf("failure = %+v, want it to say there is no terminal", f)
	}
}

// An interactive shell takes the terminal's foreground for its job control.
// Once it is gone, this process has the terminal back rather than being left in
// the background of it. Needs a terminal this process controls, so it is
// skipped where there is none.
func TestAHandoverGivesTheTerminalBack(t *testing.T) {
	tty, err := os.OpenFile(controllingTerminal, os.O_RDWR, 0)
	if err != nil {
		t.Skip("no controlling terminal")
	}
	defer tty.Close()
	if _, err := unix.IoctlGetInt(int(tty.Fd()), unix.TIOCGPGRP); err != nil {
		t.Skip("no controlling terminal")
	}
	h := sh.Terminal(Script{Shell: "bash -i -c true"}, Env(os.Environ()))

	if err := h.Run(); err != nil {
		t.Fatal(err)
	}

	fg, err := unix.IoctlGetInt(int(tty.Fd()), unix.TIOCGPGRP)
	if err != nil {
		t.Fatal(err)
	}
	if fg != unix.Getpgrp() {
		t.Errorf("foreground = %d, want this process's group %d back", fg, unix.Getpgrp())
	}
}
