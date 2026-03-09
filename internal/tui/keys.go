package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Quit         key.Binding
	Tab          key.Binding
	NewConv      key.Binding
	DeleteConv   key.Binding
	Send         key.Binding
	Approve      key.Binding
	Reject       key.Binding
	Help         key.Binding
	Escape       key.Binding
}

var keys = keyMap{
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "quit"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch pane"),
	),
	NewConv: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new conversation"),
	),
	DeleteConv: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "delete conversation"),
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
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back"),
	),
}
