package ui

import "charm.land/bubbles/v2/key"

type KeyMap struct {
	Quit    key.Binding
	Help    key.Binding
	Back    key.Binding
	Search  key.Binding
	Refresh key.Binding

	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Top      key.Binding
	Bottom   key.Binding

	Enter  key.Binding
	Insert key.Binding

	React       key.Binding
	RemoveReact key.Binding
	Reply       key.Binding
	Yank         key.Binding
	Unread       key.Binding
	Filter       key.Binding
	ToggleLayout key.Binding
}

var Keys = KeyMap{
	Quit:    key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Back:    key.NewBinding(key.WithKeys("escape", "ctrl+["), key.WithHelp("esc", "back")),
	Search:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
	Refresh: key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("ctrl+l", "refresh")),

	Up:       key.NewBinding(key.WithKeys("up", "k", "ctrl+p"), key.WithHelp("k/↑", "up")),
	Down:     key.NewBinding(key.WithKeys("down", "j", "ctrl+n"), key.WithHelp("j/↓", "down")),
	PageUp:   key.NewBinding(key.WithKeys("ctrl+u", "ctrl+b"), key.WithHelp("ctrl+u/b", "page up")),
	PageDown: key.NewBinding(key.WithKeys("ctrl+d", "ctrl+f"), key.WithHelp("ctrl+d/f", "page down")),
	Top:      key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
	Bottom:   key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),

	Enter:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
	Insert: key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "compose")),

	React:       key.NewBinding(key.WithKeys("+"), key.WithHelp("+", "react")),
	RemoveReact: key.NewBinding(key.WithKeys("-"), key.WithHelp("-", "unreact")),
	Reply:       key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reply")),
	Yank:        key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yank")),
	Unread:      key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "unread only")),
	Filter:       key.NewBinding(key.WithKeys("f", "tab"), key.WithHelp("f/tab", "filter")),
	ToggleLayout: key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "toggle sidebar")),
}
