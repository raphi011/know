package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Quit    key.Binding
	Escape  key.Binding
	Send    key.Binding
	Approve key.Binding
	Reject  key.Binding
}

var keys = keyMap{
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "quit"),
	),
	Escape: key.NewBinding(
		key.WithKeys("escape"),
		key.WithHelp("esc", "clear"),
	),
	Send: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "send message"),
	),
	Approve: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "approve"),
	),
	Reject: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "reject"),
	),
}
