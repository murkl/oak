package tui

import (
	"strings"
	"time"

	"github.com/murkl/oak/internal/wlan"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// networkScreen is where a module gets onto the internet, in one of two
// places.
//
// In the opening it stands in front of the work of a module that cannot do
// without the internet, for as long as there is none: it says so, offers the
// wireless networks in range where the machine has a card, and looks again by
// itself every few seconds, so a cable plugged in carries on without a key.
// There is no carrying on without — the work behind it would only stop at its
// first download.
//
// In the settings it is the row that joins a wireless network, taken whenever
// somebody wants one — see newJoin. Nothing is checked first and nothing is
// waited for: it looks for networks at once, and every way out is back.
type networkScreen struct {
	app   *app
	radio *wlan.Radio

	// asked is whether somebody chose to join a network from the settings,
	// rather than the opening putting the page in front of the work.
	asked bool

	step  netStep
	list  *picker
	input textinput.Model

	// card is whether the last look found a wireless device, which is what
	// decides whether pressing r means looking for networks again.
	card bool

	// round is which of the page's looks at the internet still counts. Every
	// new one starts a round, so a clock set before the page moved on is
	// ignored rather than answered twice.
	round int

	dev  string
	ssid string
	busy string
	err  string
}

type netStep int

const (
	netChecking netStep = iota
	netScanning
	netChoosing
	netPassphrase
	netJoining
	netOffline // nothing joined: no card, no networks, or backed out of the list
)

// netEvery is how often the opening looks at the internet again by itself:
// often enough that a cable plugged in carries on before anybody reaches for
// r, rarely enough that a check going out to the network is not a load of its
// own.
const netEvery = 5 * time.Second

func newNetwork(a *app, r *wlan.Radio) *networkScreen {
	return &networkScreen{app: a, radio: r}
}

// newJoin is the page the settings' row opens.
func newJoin(a *app, r *wlan.Radio) *networkScreen {
	return &networkScreen{app: a, radio: r, asked: true}
}

// In the opening it stands under the opening's heading like the pages beside
// it. Asked for from the settings, it is a page under them like any value.
func (s *networkScreen) crumbRoot() bool { return !s.asked }

func (s *networkScreen) crumbHead() string {
	if s.asked {
		return ""
	}
	return labelOpening()
}

// Title is what the page is about: the internet the work needs, or the
// wireless network somebody asked to join.
func (s *networkScreen) Title() string {
	if s.asked {
		return labelNetwork()
	}
	return labelInternet()
}

// working is what puts the turning mark in the header while a check, a scan
// or a join is in flight. Nothing is answerable while one is: the keys that
// leave are the model's, and this page has nothing else to say.
func (s *networkScreen) working() bool {
	switch s.step {
	case netChecking, netScanning, netJoining:
		return true
	}
	return false
}

// takesText: the passphrase, which is a box like any other and may hold any
// letter there is.
func (s *networkScreen) takesText() bool { return s.step == netPassphrase }

// waits reports whether the page looks at the internet by itself: in the
// opening, wherever somebody is only reading. Not while a passphrase is typed
// — a page that moved on under the hands typing it would take the next key.
func (s *networkScreen) waits() bool {
	return !s.asked && (s.step == netChoosing || s.step == netOffline)
}

type (
	netOnlineMsg struct {
		ok    bool
		round int
	}
	netDueMsg     struct{ round int }
	netScannedMsg struct {
		device string
		names  []string
		err    error
	}
	netJoinedMsg struct{ err error }
)

func (s *networkScreen) Init() tea.Cmd {
	if s.asked {
		return s.scan()
	}
	s.step, s.busy, s.err = netChecking, labelNetworkChecking(), ""
	return s.check()
}

// check asks whether there is internet, as a new round.
func (s *networkScreen) check() tea.Cmd {
	s.round++
	r, round := s.radio, s.round
	return func() tea.Msg { return netOnlineMsg{ok: r.Online(), round: round} }
}

// wait sets the clock for the next look, as a new round, wherever the page
// looks by itself at all.
func (s *networkScreen) wait() tea.Cmd {
	if !s.waits() {
		return nil
	}
	s.round++
	round := s.round
	return after(netEvery, func(time.Time) tea.Msg { return netDueMsg{round: round} })
}

func (s *networkScreen) Update(msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {

	case netOnlineMsg:
		// Answered only by the page's first look and while it waits: a look
		// that lands while a passphrase is typed would move the page on under
		// the hands typing it.
		if msg.round != s.round || (s.step != netChecking && !s.waits()) {
			return s, nil
		}
		if msg.ok {
			return s, reset(s.app.afterNetwork())
		}
		// The page's first look: offline, so what there is to offer comes
		// next. A later one found nothing either, and the clock is set again.
		if s.step == netChecking {
			if s.radio.Joinable() {
				return s, s.scan()
			}
			s.step = netOffline
		}
		return s, s.wait()

	case netDueMsg:
		if msg.round != s.round || !s.waits() {
			return s, nil
		}
		return s, s.check()

	case netScannedMsg:
		return s, s.scanned(msg)

	case netJoinedMsg:
		if msg.err != nil {
			// Almost always the passphrase. Back to the list rather than out of
			// the page: the next thing to try is another go at it.
			s.step, s.err = netChoosing, msg.err.Error()
			return s, s.wait()
		}
		s.app.joined = s.ssid
		return s, tea.Batch(recheck(), s.leave())

	case tea.KeyMsg:
		return s, s.key(msg)
	}
	return s, nil
}

// scanned lays out what the look for networks found: the list, or the reason
// there is none to show.
func (s *networkScreen) scanned(msg netScannedMsg) tea.Cmd {
	s.card = msg.device != ""
	switch {
	case msg.err != nil:
		s.step, s.err = netOffline, msg.err.Error()
	case !s.card:
		// No card is nothing to report in the opening, whose page is about the
		// internet and says to plug in a cable. Asked for from the settings, it
		// is the whole of the answer.
		s.step, s.err = netOffline, ""
		if s.asked {
			s.err = labelNetworkNoDevice()
		}
	case len(msg.names) == 0:
		s.step, s.err = netOffline, labelNetworkNoNetworks()
	default:
		s.dev = msg.device
		items := make([]item, len(msg.names))
		for i, n := range msg.names {
			items[i] = item{title: n, key: n}
		}
		s.list = newPicker(items)
		s.list.describe(s.help())
		s.step, s.err = netChoosing, ""
	}
	return s.wait()
}

func (s *networkScreen) key(k tea.KeyMsg) tea.Cmd {
	switch s.step {

	case netChoosing:
		switch {
		case confirms(k):
			ssid, ok := s.list.chosen()
			if !ok {
				return nil
			}
			s.ssid, s.step, s.err = ssid, netPassphrase, ""
			s.input = textinput.New()
			s.input.EchoMode = textinput.EchoPassword
			s.input.EchoCharacter = []rune(glyphs.secret)[0]
			s.input.CharLimit = 128
			styleInput(&s.input)
			s.input.Focus()
			return textinput.Blink
		case backs(k):
			// Asked for, back is back. In the opening it is one step back
			// rather than out of the page: what is behind the list is the page
			// saying there is no connection, which still carries on by itself
			// the moment there is one.
			if s.asked {
				return pop()
			}
			s.step, s.err = netOffline, ""
			return nil
		case k.String() == "r":
			return s.scan()
		}
		s.list.Update(k)
		return nil

	case netPassphrase:
		switch {
		case confirms(k):
			s.step, s.busy = netJoining, labelNetworkJoining(s.ssid)
			r, dev, ssid, pass := s.radio, s.dev, s.ssid, s.input.Value()
			return func() tea.Msg { return netJoinedMsg{err: r.Join(dev, ssid, pass)} }
		// Esc alone: backspace is the delete key in front of a passphrase.
		case cancels(k):
			s.step, s.err = netChoosing, ""
			return s.wait()
		}
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(k)
		return cmd

	case netOffline:
		switch {
		case k.String() == "r":
			if s.asked {
				return s.scan()
			}
			return s.Init()
		case backs(k):
			// Out of the page: to the settings, or to whatever the opening
			// asked before it.
			return pop()
		}
	}
	return nil
}

// leave is where this page hands over once a network is joined: on into the
// opening, or back to the settings it was opened from.
func (s *networkScreen) leave() tea.Cmd {
	if s.asked {
		return pop()
	}
	return reset(s.app.afterNetwork())
}

func (s *networkScreen) scan() tea.Cmd {
	s.step, s.busy, s.err = netScanning, labelNetworkScanning(), ""
	r := s.radio
	return func() tea.Msg {
		dev, err := r.Device()
		if err != nil || dev == "" {
			return netScannedMsg{err: err}
		}
		names, err := r.Networks(dev)
		return netScannedMsg{device: dev, names: names, err: err}
	}
}

// help is the sentence over the list of networks.
func (s *networkScreen) help() string {
	if s.asked {
		return labelNetworkJoin()
	}
	return labelNetworkHelp()
}

func (s *networkScreen) View(width, height int) string {
	switch s.step {
	case netChecking, netScanning, netJoining:
		return accentStyle.Render(spinFrame()) + field(" ") + boldStyle.Render(s.busy)

	case netChoosing:
		if s.err == "" {
			return s.list.View(width, height)
		}
		return s.list.View(width, height-2) + "\n\n" + failStyle.Render(truncate(s.err, width))

	case netPassphrase:
		s.input.Width = width - lipgloss.Width(glyphs.cursor) - 1
		var b strings.Builder
		b.WriteString(boldStyle.Render(s.ssid) + "\n")
		b.WriteString(softStyle.Render(labelPassphrase()) + "\n")
		b.WriteString(cursorStyle.Render(glyphs.cursor) + s.input.View())
		return b.String()
	}

	// netOffline
	body := s.err
	if body == "" {
		body = labelNetworkOffline()
	}
	out := failStyle.Render(glyphs.fail) + field(" ") + boldStyle.Render(body)
	// Asked for, what went wrong is the page. In the opening, the sentence under
	// it says what gets the work going.
	if s.asked {
		return out
	}
	wait := labelNetworkWait()
	if s.card {
		wait = labelNetworkWaitWireless()
	}
	return out + "\n\n" + paragraph(wait, width)
}

func (s *networkScreen) Hint() string {
	switch s.step {
	case netChecking, netScanning, netJoining:
		return labelHintRunning()
	case netChoosing:
		return labelHintNetworkChoosing()
	case netPassphrase:
		return labelHintInput()
	}
	return labelHintNetworkRetry()
}
