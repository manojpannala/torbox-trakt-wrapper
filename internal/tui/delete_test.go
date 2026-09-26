package tui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
)

type deleteRecorder struct {
	mu    sync.Mutex
	calls []map[string]any
}

func (r *deleteRecorder) record(req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	var decoded map[string]any
	_ = json.Unmarshal(body, &decoded)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, decoded)
}

func (r *deleteRecorder) last() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.calls) == 0 {
		return nil
	}
	return r.calls[len(r.calls)-1]
}

func torboxControlStub(t *testing.T) *deleteRecorder {
	t.Helper()
	rec := &deleteRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("TORBOX_BASE_URL", srv.URL)
	return rec
}

func TestDelete_TargetsTheItemConfirmedNotWhateverIsAtTheCursor(t *testing.T) {
	rec := torboxControlStub(t)
	m := testModel(t)
	m, _ = sendMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = sendMsg(m, TorrentsLoadedMsg{Torrents: []torbox.Torrent{
		{ID: 1, Name: "Test.Feature.Alpha.2023.1080p.mkv"},
		{ID: 2, Name: "Test.Feature.Beta.2022.2160p.mkv"},
	}})
	m, _ = sendKey(m, downKey)
	require.Equal(t, 2, m.selectedCurrentItem().ID, "cursor must be on the second torrent before the modal opens")

	m, _ = sendKey(m, tea.KeyPressMsg{Code: 'd', Text: "d"})
	require.Equal(t, ModalDelete, m.activeModal)

	m, _ = sendMsg(m, TorrentsLoadedMsg{Torrents: []torbox.Torrent{
		{ID: 3, Name: "Test.Series.Gamma.S01E01.mkv"},
		{ID: 1, Name: "Test.Feature.Alpha.2023.1080p.mkv"},
		{ID: 2, Name: "Test.Feature.Beta.2022.2160p.mkv"},
	}})

	_, cmd := sendKey(m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	require.NotNil(t, cmd)
	cmd()

	body := rec.last()
	require.NotNil(t, body, "the delete must reach the control endpoint")
	require.Equal(t, float64(2), body["torrent_id"], "the item confirmed at 'd' time must be the one deleted, not whatever the reconcile put at the cursor")
}
