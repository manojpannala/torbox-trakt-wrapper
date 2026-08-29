package tui_test

import (
	"testing"

	"github.com/manojpannala/torbox-trakt-wrapper/internal/tui"
	"github.com/stretchr/testify/assert"
)

func TestTheme_Default(t *testing.T) {
	theme := tui.DefaultTheme()
	assert.NotEmpty(t, theme.AppTitle.Render("Test App"))
	assert.NotEmpty(t, theme.TabActive.Render("Tab 1"))
	assert.NotEmpty(t, theme.TabInactive.Render("Tab 2"))
	assert.NotEmpty(t, theme.BadgeWatched.Render("✓"))
	assert.NotEmpty(t, theme.BadgeInProgress.Render("◐"))
	assert.NotEmpty(t, theme.ModalBox.Render("Modal Content"))
}
