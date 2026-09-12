package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestClipboardPrefill(t *testing.T) {
	for _, tc := range []struct {
		name string
		clip string
		want bool
	}{
		{"magnet", "magnet:?xt=urn:btih:abc", true},
		{"https", "https://example.invalid/a.nzb", true},
		{"http", "http://example.invalid/a.mp4", true},
		{"prose is not a link", "remember to buy milk", false},
		{"empty", "", false},
		{"a password manager entry", "hunter2", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := clipboardPrefill(tc.clip)
			assert.Equal(t, tc.want, ok)
		})
	}
}

func TestAddModal_SaysWhenTheValueCameFromTheClipboard(t *testing.T) {
	m := NewAddModal()
	m.Input.SetValue("magnet:?xt=urn:btih:abc")
	m.FromClipboard = true

	out := m.Render(DefaultTheme(), 120)

	assert.Contains(t, out, "clipboard", "a value the user did not type must say so")
	assert.Contains(t, out, "check it")
}

func TestAddModal_SaysNothingWhenTheUserTypedIt(t *testing.T) {
	m := NewAddModal()

	out := m.Render(DefaultTheme(), 120)

	assert.NotContains(t, out, "clipboard")
	assert.Contains(t, out, "Paste magnet link")
}

func TestAddModal_TheNoticeClearsOnceTheUserEdits(t *testing.T) {
	m := AppModel{activeModal: ModalAdd, addModal: NewAddModal()}
	m.addModal.Input.SetValue("magnet:?xt=urn:btih:abc")
	m.addModal.FromClipboard = true

	next, _ := m.handleKeyMsg(tea.KeyPressMsg{Code: 'x', Text: "x"})

	assert.False(t, next.(AppModel).addModal.FromClipboard,
		"editing makes it the user's own value")
}

func TestAddModal_ShowsTheStartOfAPastedValue(t *testing.T) {
	// "check it before adding" is meaningless if the scheme is scrolled off.
	long := "magnet:?xt=urn:btih:0123456789abcdef&dn=Some.Very.Long.Movie.Name.2023.2160p"

	m := AppModel{activeModal: ModalNone, addModal: NewAddModal()}
	m.addModal = prefilledAddModal(long)

	assert.Contains(t, m.addModal.Render(DefaultTheme(), 120), "magnet:?",
		"the user must be able to see what scheme they are confirming")
}
