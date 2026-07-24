package sectioneditor

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/theme"
)

func newTestModel() Model {
	ctx := &context.ProgramContext{Theme: *theme.DefaultTheme}
	limit := 10
	return New(ctx, "Mine", config.SectionConfig{
		Filters:     "is:open author:@me",
		Limit:       &limit,
		ExtraFields: "state",
		LocalFilter: `state == "OPEN"`,
	})
}

func TestNew_PrefillsFieldsAndFocusesTitle(t *testing.T) {
	m := newTestModel()

	title, filters, limit, extraFields, localFilter := m.Values()
	require.Equal(t, "Mine", title)
	require.Equal(t, "is:open author:@me", filters)
	require.Equal(t, "10", limit)
	require.Equal(t, "state", extraFields)
	require.Equal(t, `state == "OPEN"`, localFilter)
	require.Equal(t, fieldTitle, m.focus)
}

func TestUpdate_TabAdvancesThroughAllFieldsAndWrapsAround(t *testing.T) {
	m := newTestModel()

	order := []fieldIndex{fieldFilters, fieldLimit, fieldExtraFields, fieldLocalFilter, fieldTitle}
	for _, want := range order {
		var action Action
		m, _, action = m.Update(tea.KeyPressMsg{Text: "tab"})
		require.Equal(t, ActionNone, action)
		require.Equal(t, want, m.focus)
	}
}

func TestUpdate_ShiftTabMovesBackwardAndWrapsAround(t *testing.T) {
	m := newTestModel()

	m, _, action := m.Update(tea.KeyPressMsg{Text: "shift+tab"})
	require.Equal(t, ActionNone, action)
	require.Equal(t, fieldLocalFilter, m.focus)
}

func TestUpdate_CtrlSReportsSubmitAction(t *testing.T) {
	m := newTestModel()

	_, _, action := m.Update(tea.KeyPressMsg{Text: "ctrl+s"})
	require.Equal(t, ActionSubmit, action)
}

func TestUpdate_EscReportsCancelAction(t *testing.T) {
	m := newTestModel()

	_, _, action := m.Update(tea.KeyPressMsg{Text: "esc"})
	require.Equal(t, ActionCancel, action)
}

func TestUpdate_TypingGoesToTheFocusedField(t *testing.T) {
	m := newTestModel()

	// New() focuses Title; tab twice reaches Limit (Title -> Filters -> Limit).
	m, _, action := m.Update(tea.KeyPressMsg{Text: "tab"})
	require.Equal(t, ActionNone, action)
	m, _, action = m.Update(tea.KeyPressMsg{Text: "tab"})
	require.Equal(t, ActionNone, action)
	require.Equal(t, fieldLimit, m.focus)

	m, _, _ = m.Update(tea.KeyPressMsg{Text: "5"})

	_, _, limit, _, _ := m.Values()
	require.Equal(t, "105", limit)
}
