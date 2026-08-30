package tui_test

import (
	"bytes"
	"io"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/internal/tui"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
)

// Under Bubble Tea v2 the alt screen is a field on the view rather than a
// program option, so a View that forgets to set it still compiles and still
// passes every render test — while scribbling over the user's scrollback. This
// runs the real program headless and checks the escape sequences on the wire.
func TestAppModel_ProgramEntersAndLeavesTheAltScreen(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	in, keys := io.Pipe()
	var out bytes.Buffer

	p := tea.NewProgram(
		tui.NewAppModel(t.Context(), config.DefaultConfig()),
		tea.WithInput(in),
		tea.WithOutput(&out),
		tea.WithWindowSize(goldenWidth, goldenHeight),
		tea.WithoutSignals(),
	)

	done := make(chan error, 1)
	go func() {
		_, err := p.Run()
		done <- err
	}()

	go func() {
		_, _ = keys.Write([]byte("q"))
		_ = keys.Close()
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		p.Kill()
		t.Fatal("program did not quit after q")
	}

	rendered := out.String()
	assert.Contains(t, rendered, "\x1b[?1049h", "program never entered the alt screen")
	assert.Contains(t, rendered, "\x1b[?1049l", "program never left the alt screen")
}
