#!/usr/bin/env python3
"""Render docs/screenshots: every page of the interface, out of the example.

The pages are taken from the real binary driven on a real pty, not mocked up,
so a screenshot cannot describe an interface that no longer exists. Each one
starts from a pristine copy of example/, because a preference written by the
page before would otherwise decide what the next picture looks like.

Every page but one comes out byte for byte the same on every run. The
exception is run.png, which catches the run while it is still going: which
task the frame lands on is a race against four tasks that take no time, so
that one picture differs from run to run. It is a true page either way.

Needs: chromium, imagemagick, python-pyte.
"""

import argparse
import fcntl
import html
import os
import pathlib
import pty
import re
import select
import shutil
import struct
import subprocess
import sys
import termios
import time

try:
    import pyte
except ModuleNotFoundError:
    sys.exit("needs python-pyte: sudo pacman -S python-pyte")

COLS, ROWS = 95, 25

# The card. One shape for every screenshot in every project that uses this, so
# a set stays one set: the grid decides the size, never the other way round.
FONT_PX, LINE_PX = 15, 22
ADVANCE = FONT_PX * 0.6  # FiraCode advances 600/1000 em, so this is exact
PAD_X, PAD_Y = 25.9, 26
RADIUS, SCALE = 10, 2
CARD, TEXT = "#2e3440", "#d8dee9"

# The `monospace` fallback is load bearing. Without it the small triangle the
# interface marks a selected row with comes out as a big filled one.
FONT = '"FiraCode Nerd Font Mono", monospace'

# The failure page prints where the log is, so the folder a capture runs in
# ends up inside a published picture. It is short and neutral for that reason
# and for no other.
WORK = pathlib.Path("/tmp/tux")

KEY = {"enter": b"\r", "down": b"\x1bOB", "up": b"\x1bOA", "esc": b"\x1b"}


class Session:
    """A program on a pty: press keys, keep every frame it drew.

    Every read is kept, because some pages only exist for a moment. Four tasks
    that do nothing are over before anything settles, and the page worth a
    picture is the one in the middle of that.
    """

    def __init__(self, argv, cwd, env=None):
        self.frames, self._buf = [], b""
        self.pid, self.fd = pty.fork()
        if self.pid == 0:
            os.chdir(cwd)
            os.execvpe(argv[0], argv, dict(
                os.environ, TERM="xterm-256color", COLORTERM="truecolor",
                LINES=str(ROWS), COLUMNS=str(COLS), **(env or {})))
        fcntl.ioctl(self.fd, termios.TIOCSWINSZ,
                    struct.pack("HHHH", ROWS, COLS, 0, 0))

    def pump(self, seconds):
        end = time.time() + seconds
        while time.time() < end:
            if not select.select([self.fd], [], [], max(0, end - time.time()))[0]:
                continue
            try:
                chunk = os.read(self.fd, 65536)
            except OSError:
                return False
            if not chunk:
                return False
            if b"\x1b[6n" in chunk:
                os.write(self.fd, b"\x1b[1;1R")  # answer as a real terminal does
            self._buf += chunk
            self.frames.append(self._buf)
        return True

    def press(self, *names, settle=0.6):
        for name in names:
            os.write(self.fd, KEY.get(name, name.encode()))
            self.pump(settle)
        return self

    def until(self, *wants, timeout=10.0):
        """Wait for a page, not for a length of time.

        The splash holds for a second and a half before a page is there to take
        a key, and a key pressed into it is a key the program never sees, which
        moves every page after it along by one.
        """
        end = time.time() + timeout
        while time.time() < end:
            if all(w in text(self._buf) for w in wants):
                return self
            if not self.pump(0.15):
                break
        raise SystemExit(f"waited {timeout}s, never saw {wants!r}")

    def still(self, checks=3, step=0.12, timeout=8.0):
        """Wait until the screen stops changing.

        The interface fades in, so a page can be readable a frame before it has
        finished arriving. A picture taken then is a shade off, every time and
        never the same shade twice.
        """
        last, same, end = None, 0, time.time() + timeout
        while time.time() < end:
            now = markup(grid(self._buf))
            same = same + 1 if now == last else 0
            last = now
            if same >= checks:
                return self
            self.pump(step)
        return self

    def screen(self):
        return self._buf

    def close(self):
        try:
            os.close(self.fd)
        except OSError:
            pass


def grid(stream):
    screen = pyte.Screen(COLS, ROWS)
    pyte.Stream(screen).feed(stream.decode("utf-8", "replace"))
    return screen


def text(stream):
    return "\n".join(grid(stream).display)


def working(session, done="✓", ended="tests passed"):
    """The run page furthest along that still has a task left to do.

    Which frames a run happens to land on is a race, so this asks for the
    fullest one there was rather than for a particular count - with a fully
    ticked list ruled out, because a run with nothing left to do is not a run
    under way.
    """
    best, ticks = None, -1
    for frame in session.frames:
        seen = text(frame)
        count = re.search(r"\d+ of (\d+)", seen)
        if ended in seen or not count:
            continue
        marks = seen.count(done)
        if marks < int(count.group(1)) and marks > ticks:
            best, ticks = frame, marks
    if best is None:
        raise SystemExit("no frame caught the run under way")
    return best


def colour(value, fallback):
    if value in (None, "default"):
        return fallback
    ok = len(value) == 6 and all(c in "0123456789abcdefABCDEF" for c in value)
    return f"#{value}" if ok else value


def markup(screen):
    """One span a run of same-looking cells, not one a character."""
    rows = []
    for y in range(screen.lines):
        line, run, style = [], "", None
        for x in range(screen.columns):
            c = screen.buffer[y][x]
            fg, bg = colour(c.fg, TEXT), colour(c.bg, None)
            if c.reverse:
                fg, bg = bg or CARD, fg
            now = (fg, bg, c.bold)
            if now != style and run:
                line.append((style, run))
                run = ""
            style, run = now, run + (c.data or " ")
        if run:
            line.append((style, run))
        rows.append("".join(
            '<span style="color:{};{}{}">{}</span>'.format(
                f, f"background:{b};" if b else "",
                "font-weight:700" if w else "", html.escape(t))
            for (f, b, w), t in line))
    return "\n".join(rows)


def card(stream, out):
    screen = grid(stream)
    w = round(screen.columns * ADVANCE + 2 * PAD_Y + 1)
    h = round(screen.lines * LINE_PX + 2 * PAD_Y)
    page = out.with_suffix(".html")
    page.write_text(f"""<!DOCTYPE html>
<html><head><meta charset="utf-8"><style>
  * {{ margin:0; padding:0; }}
  html,body {{ width:{w}px; height:{h}px; background:transparent; }}
  .card {{ position:absolute; inset:0; background:{CARD}; border-radius:{RADIUS}px; }}
  pre {{
    position:absolute; left:{PAD_X}px; top:{PAD_Y}px;
    font-family:{FONT}; font-size:{FONT_PX}px; line-height:{LINE_PX}px;
    color:{TEXT}; white-space:pre; font-variant-ligatures:none;
  }}
</style></head><body><div class="card"></div><pre>{markup(screen)}</pre></body></html>""")
    try:
        subprocess.run([
            "chromium", "--headless", f"--force-device-scale-factor={SCALE}",
            "--default-background-color=00000000", "--hide-scrollbars",
            f"--window-size={w},{h}", f"--screenshot={out}", page.as_uri(),
        ], check=True, capture_output=True)
    finally:
        page.unlink(missing_ok=True)
    subprocess.run(["magick", str(out), "-strip",
                    "-define", "png:compression-level=9", str(out)], check=True)
    return out


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--example", type=pathlib.Path, default=pathlib.Path("example"))
    p.add_argument("--binary", type=pathlib.Path, required=True)
    p.add_argument("--out", type=pathlib.Path, default=pathlib.Path("docs/screenshots"))
    args = p.parse_args()

    binary = args.binary.resolve()
    source = args.example.resolve()
    out = args.out.resolve()
    out.mkdir(parents=True, exist_ok=True)

    def session():
        shutil.rmtree(WORK, ignore_errors=True)
        shutil.copytree(source, WORK)
        shutil.copy(binary, WORK / "oak")
        (WORK / "oak").chmod(0o755)
        s = Session(["./oak"], str(WORK), env={"LANG": "C.UTF-8", "LC_ALL": "C.UTF-8"})
        s.pump(1.0)
        return s

    def shot(name, stream):
        card(stream, out / f"{name}.png")
        print(f"  {name}")

    print(f"-> {out}")
    try:
        # the page every run opens on, and the page behind it
        s = session()
        shot("welcome", s.until("Please choose the language").still().screen())
        s.press("enter")
        shot("choice", s.until("What to do").still().screen())
        s.close()

        # one question, on a page of its own
        s = session()
        s.until("Please choose the language").press("enter")
        s.until("What to do").press("down")
        s.until("Build a small Linux").press("enter")
        shot("question", s.until("Hostname").still().screen())
        s.close()

        # the run under way, and the page it stops on
        s = session()
        s.until("Please choose the language").press("enter")
        s.until("What to do").press("down")
        s.until("Build a small Linux").press("enter")
        s.until("Hostname").press("enter")
        s.until("User name").press("enter")
        s.until("Desktop").press("enter")
        s.until("Start", "Settings").press("enter")
        s.until("Ready to start").press("enter", settle=0.1)
        # Let the whole run go by first, then go back through the frames for
        # the one worth keeping. Choosing while it is still going only ever
        # finds whatever had happened by then.
        s.until("tests passed", "continue")
        shot("run", working(s))
        shot("report", s.still().screen())
        s.close()

        # a task that fails: recovery checks a tree that was never built. The
        # page worth keeping is the one behind it, which names the script, the
        # line, the command and what it exited with.
        s = session()
        s.until("Please choose the language").press("enter")
        s.until("What to do").press("enter")
        s.until("Start", "Settings").press("enter")
        s.until("Ready to start").press("enter")
        s.until("Failed").press("enter")
        shot("failure", s.until("Exit code").still().screen())
        s.close()
    finally:
        shutil.rmtree(WORK, ignore_errors=True)


if __name__ == "__main__":
    sys.exit(main())
