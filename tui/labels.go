package tui

import "github.com/murkl/oak/internal/i18n"

// Every word the interface says that does not come out of a module: the fixed
// pages, the buttons, the key hints. They are here as functions
// rather than constants because the language can change while the program is
// running — the first screen is the one that changes it — and a constant read
// at package init would be the one word left in the old language.
//
// The English text is the message and also its own key; see internal/i18n.

// say is the interface's own voice: the message in the language showing, in
// marks the terminal can actually draw. Every word below goes through it,
// because the ones that need rewriting are as likely to arrive from a catalog
// as from the line above — a translation of "⏎ continue" carries the same
// symbol, and a console can draw it no better in German.
func say(msg string, args ...any) string {
	return glyphs.spell.Replace(i18n.T(msg, args...))
}

func labelHintMenu() string     { return say("↑↓ move · ⏎ select · q quit") }
func labelHintList() string     { return say("↑↓ move · ⏎ open · esc back") }
func labelHintChoose() string   { return say("↑↓ move · ⏎ confirm · esc back") }
func labelHintInput() string    { return say("⏎ confirm · esc back") }
func labelHintRunning() string  { return say("working …") }
func labelHintContinue() string { return say("⏎ continue") }
func labelChecking() string     { return say("Checking this machine …") }
func labelHintBack() string     { return say("⏎ back") }
func labelHintClose() string    { return say("⏎ close") }
func labelHintQuit() string     { return say("⏎ quit") }
func labelHintStart() string    { return say("⏎ start · esc back") }

// labelHintAnswer is the hint for a question a run stopped to ask. It is the
// one list in the program with nothing behind it — the task waiting on the
// answer has already started — so esc there says what q says on the menu.
func labelHintAnswer() string { return say("↑↓ move · ⏎ confirm · esc quit") }

// The network screen: checking for internet, and — where the module describes
// how — joining a wireless one.
func labelNetwork() string         { return say("Wireless network") }
func labelNetworkHelp() string     { return say("Join a wireless network to continue.") }
func labelNetworkChecking() string { return say("Checking the internet connection …") }
func labelNetworkScanning() string { return say("Looking for wireless networks …") }

// TRANSLATORS: %s is the name of the wireless network being joined.
func labelNetworkJoining(ssid string) string {
	return say("Joining %s …", ssid)
}
func labelNetworkOffline() string    { return say("There is no internet connection.") }
func labelNetworkNoNetworks() string { return say("No wireless networks in range.") }
func labelPassphrase() string        { return say("Passphrase") }
func labelContinueAnyway() string    { return say("Continue anyway") }
func labelHintNetworkChoosing() string {
	return say("↑↓ move · ⏎ join · r rescan · esc skip")
}
func labelHintNetworkOffline() string { return say("⏎ continue · r retry · esc back") }
func labelNetworkOfflineHelp() string {
	return say("Everything that gets installed is downloaded. Plug in a cable, or press r to search again.")
}
func labelNetworkOfflineHelpUnjoinable() string {
	return say("Everything that gets installed is downloaded. Plug in a cable, or join a wireless network before continuing.")
}

// The narrowing box: the key that opens it, appended to a list's own hint, and
// what the keys mean while it is open.
func labelHintFilterKey() string { return say("/ filter") }
func labelHintFilter() string {
	return say("type to filter · ↑↓ move · ⏎ select · esc close")
}

// labelHintFilterPermanent is the permanent box's own hint: esc means back
// rather than close, because there is no box left to close first.
func labelHintFilterPermanent() string {
	return say("type to filter · ↑↓ move · ⏎ select · esc back")
}

func labelFilterPlaceholder() string { return say("Filter …") }
func labelNoMatch() string           { return say("No matches") }

// The sign-off under the wordmark on the splash, and the one place Oak names
// itself: which program drew this product, and which build of it.

// TRANSLATORS: %s is Oak's own version, for example "v1.4.0".
func labelPoweredBy(version string) string { return say("powered by oak %s", version) }

// The heading over the opening: the pages in front of the questions proper.
// They come one after another rather than one inside the other, so the line
// above them says which part of the program this is and then which of its pages
// — never the row of answers already given.
func labelOpening() string { return say("Start") }

func labelLanguage() string { return say("Interface language") }

// The sentence under it, and it says what the setting does not do: a machine
// being installed has a language of its own, and somebody who has just been
// asked for one twice is owed the difference in plain words.

// TRANSLATORS: %s is the name of the product being read, as oak.yaml declares it.
func labelLanguageHelp(name string) string {
	return say("The language %s is read in. It changes the words on screen and nothing else.", name)
}

// The fork after it, where a runtime offers more than one module. Only the
// page's own name is the runtime's: what is on it, and what each of them is, is
// said in each module's own words.
func labelChoice() string { return say("What to do") }

// labelCounter is where something sits in a run of things: which question of
// how many, which task of how many. Bare numbers, because it is read in the
// header beside what it is counting — a word in front of it would only repeat
// what the page already says.
func labelCounter(at, of int) string { return say("%d of %d", at, of) }

func labelSettings() string { return say("Settings") }

// What a run proved about itself: how many of the tests its tasks declared the
// machine agreed with. It is read twice — under the line that says the run is
// over, and again as the heading of the page listing the ones it did not.
//
// One sentence for both outcomes, because what is in front of it already says
// which of the two this is: a mark on that page, and the colour of the line
// under the run.

// TRANSLATORS: the first %d is how many tests passed, the second how many ran.
func labelTestsPassed(passed, ran int) string { return say("%d of %d tests passed", passed, ran) }

func labelValidation() string { return say("Validation") }
func labelHintChecks() string { return say("↑↓ move · ⏎ open · esc continue") }

// The switch in the settings, and the heading it stands under. It is the
// runtime's own answer and holds for every module: what a task tests is the
// module's business, whether anything is tested at all is not.
//
// The heading is what is being decided and the row is what it is being decided
// about, so the two read as one line: validate — the installation scripts. The
// sentence under it says what saying yes is worth, because somebody reading it
// is deciding whether a thing they have never seen fail is worth the time.
func labelValidating() string        { return say("Validate") }
func labelValidatingScripts() string { return say("Installation scripts") }
func labelValidatingHelp() string {
	return say("Reads the machine after every task: that what the task installed is really there, and set up the way it was meant to be. It is what makes an installation you can rely on rather than one that only said it worked. Nothing is changed and nothing is stopped. Whatever disagrees is read at the end.")
}

// TRANSLATORS: %s is the name of the module whose values these are.
func labelSettingsHelp(name string) string {
	return say("Every value %s will use. Choose one to change it.", name)
}

func labelPasswordRepeat() string   { return say("Repeat") }
func labelPasswordMismatch() string { return say("The entries do not match.") }

// What a run is called is the module's own title, and the runtime supplies the
// sentence around it and nothing else. It has no name of its own to fall back
// on: whether this module installs anything is not something it knows.
//
// The clock is on three of them. How long a run has been going is the one thing
// somebody watching a list of tasks actually wants to know and cannot work out
// for themselves, and how long it took is the same answer once it is over.
func labelReadyToStart() string          { return say("Ready to start") }
func labelStartNamed(name string) string { return say("Start %s", name) }
func labelRunFailed(name string) string  { return say("%s failed", name) }
func labelRunningFor(name, elapsed string) string {
	return say("%s · %s", name, elapsed)
}
func labelRunDone(name, elapsed string) string {
	return say("%s complete in %s", name, elapsed)
}
func labelLogHint(path string) string {
	return say("The full log is in %s.", path)
}

// What the page a run stopped on says under the mark. The headline already
// names the run; this names the step it got to and what that means for
// everything after it.
//
// TRANSLATORS: %s is the name of the step the run stopped at.
func labelRunStopped(step string) string {
	return say("It stopped at %s, and nothing after that has run.", step)
}

// What a question put in the middle of a run says when the answers to it turn
// out to be none. A run cannot go on past it — the value it was waiting for
// does not exist on this machine — so it reads as the failure it is.
func labelNothingToChoose(title string) string {
	return say("%s: there is nothing to choose from.", title)
}

func labelCannotContinue() string { return say("Cannot continue") }

// The way out, on a machine where leaving is not quitting a program but
// deciding what happens to the machine — see leave.go.
func labelLeave() string { return say("Leave") }

// Read over the rows when this page went up in the middle of a run: both halves
// of what somebody who pressed esc during an installation needs to know.
func labelLeaveRunning() string {
	return say("This run continues behind this page. Any choice here stops it.")
}

func labelRestart() string      { return say("Restart") }
func labelRestartHelp() string  { return say("Close this machine down and start it again.") }
func labelShutdown() string     { return say("Shut down") }
func labelConsole() string      { return say("Exit") }
func labelShutdownHelp() string { return say("Switch this machine off.") }
func labelRestarting() string   { return say("Restarting …") }
func labelShuttingDown() string { return say("Shutting down …") }
func labelLeaveFailed() string  { return say("The machine did not respond.") }

// The two answers to a task that asks before it runs. The same two words a bool
// is read out in, because they are the same question.
func labelYes() string { return say("Yes") }
func labelNo() string  { return say("No") }
