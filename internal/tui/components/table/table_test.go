package table

import (
	"os"
	"testing"
	"time"

	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/constants"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/theme"
)

// TestMain initializes the bubblezone manager once for this package's
// tests: renderRow calls zone.Mark unconditionally (to make list rows
// clickable), which panics if no manager was ever set up.
func TestMain(m *testing.M) {
	zone.NewGlobal()
	os.Exit(m.Run())
}

func newTestTable(t *testing.T, rows []Row) Model {
	t.Helper()

	cfg, err := config.ParseConfig(config.Location{
		ConfigFlag:       "../../../config/testdata/test-config.yml",
		SkipGlobalConfig: true,
	})
	require.NoError(t, err)

	thm := theme.ParseTheme(&cfg)
	ctx := context.ProgramContext{
		Config: &cfg,
		Theme:  thm,
		Styles: context.InitStyles(thm),
	}

	m := NewModel(
		ctx,
		constants.Dimensions{Width: 60, Height: 10},
		time.Now(),
		time.Now(),
		[]Column{{Title: "Title"}},
		rows,
		"pr",
		nil,
		"Loading...",
		false,
	)
	m.SetRows(rows)

	return m
}

func TestSetCurrRow_ChangesSelection(t *testing.T) {
	m := newTestTable(t, []Row{{"first"}, {"second"}, {"third"}})
	require.Equal(t, 0, m.GetCurrItem())

	got := m.SetCurrRow(2)

	require.Equal(t, 2, got)
	require.Equal(t, 2, m.GetCurrItem())
}

func TestSetCurrRow_ClampsOutOfRangeValues(t *testing.T) {
	m := newTestTable(t, []Row{{"first"}, {"second"}})

	require.Equal(t, 1, m.SetCurrRow(99), "must clamp to the last row")
	require.Equal(t, 0, m.SetCurrRow(-5), "must clamp to the first row")
}

func TestSetCurrRow_UpdatesTheHighlightedRowOnNextRender(t *testing.T) {
	// Regression test: the selected-row style is decided inside renderRow
	// at SyncViewPortContent time, not dynamically in View() - SetCurrRow
	// must trigger a re-sync so the visual highlight actually moves.
	m := newTestTable(t, []Row{{"first"}, {"second"}})

	before := m.View()
	m.SetCurrRow(1)
	after := m.View()

	require.NotEqual(t, before, after, "selecting a different row must change the rendered output")
}

func TestRenderRow_RegistersAClickableZonePerRow(t *testing.T) {
	m := newTestTable(t, []Row{{"first"}, {"second"}, {"third"}})

	zone.Scan(m.View())

	// zone.Scan() buffers zone info asynchronously (documented on Manager.Scan) -
	// Get() may briefly lag behind, so poll instead of asserting immediately.
	for _, id := range []string{"row-0", "row-1", "row-2"} {
		require.Eventually(t, func() bool {
			return !zone.Get(id).IsZero()
		}, time.Second, time.Millisecond, "%s must be a registered zone", id)
	}
}
