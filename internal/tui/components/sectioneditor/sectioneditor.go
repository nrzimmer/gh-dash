// Package sectioneditor implements the multi-field form opened by Ctrl+E to
// edit a config-backed section's filters/limit/extraFields/localFilter
// without leaving the TUI or hand-editing config.yml.
package sectioneditor

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/dlvhdr/gh-dash/v4/internal/config"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/components/inputbox"
	"github.com/dlvhdr/gh-dash/v4/internal/tui/context"
)

type fieldIndex int

const (
	fieldTitle fieldIndex = iota
	fieldFilters
	fieldLimit
	fieldExtraFields
	fieldLocalFilter
	fieldCount
)

var fieldLabels = [fieldCount]string{
	fieldTitle:       "Title",
	fieldFilters:     "Filters",
	fieldLimit:       "Limit",
	fieldExtraFields: "Extra Fields (GraphQL)",
	fieldLocalFilter: "Local Filter (expr)",
}

// Action reports what a keypress asked the form to do, since Update alone
// (which only handles moving focus between fields) can't signal submit/
// cancel back to the caller the way a plain tea.Cmd would.
type Action int

const (
	ActionNone Action = iota
	ActionSubmit
	ActionCancel
)

type Model struct {
	ctx    *context.ProgramContext
	Title  string // the section's title when the editor was opened - always shown in the header, even if the Title field is edited
	fields [fieldCount]inputbox.Model
	focus  fieldIndex
}

// New builds a section editor prefilled from cfg. title is the section's
// current title; unlike the other fields it also doubles as the header
// label, since renaming happens through the Title field but the header
// keeps identifying which section is being edited.
func New(ctx *context.ProgramContext, title string, cfg config.SectionConfig) Model {
	titleInput := inputbox.DefaultTextInput(ctx)
	titleInput.SetValue(title)

	filtersInput := inputbox.DefaultTextInput(ctx)
	filtersInput.SetValue(cfg.Filters)

	limitInput := inputbox.DefaultTextInput(ctx)
	if cfg.Limit != nil {
		limitInput.SetValue(strconv.Itoa(*cfg.Limit))
	}

	extraFieldsArea := inputbox.DefaultTextArea(ctx)
	extraFieldsArea.SetValue(cfg.ExtraFields)

	localFilterArea := inputbox.DefaultTextArea(ctx)
	localFilterArea.SetValue(cfg.LocalFilter)

	m := Model{
		ctx:   ctx,
		Title: title,
		focus: fieldTitle,
	}
	m.fields[fieldTitle] = inputbox.NewModel(ctx, inputbox.ModelOpts{TextInput: &titleInput})
	m.fields[fieldFilters] = inputbox.NewModel(ctx, inputbox.ModelOpts{TextInput: &filtersInput})
	m.fields[fieldLimit] = inputbox.NewModel(ctx, inputbox.ModelOpts{TextInput: &limitInput})
	m.fields[fieldExtraFields] = inputbox.NewModel(ctx, inputbox.ModelOpts{TextArea: &extraFieldsArea})
	m.fields[fieldLocalFilter] = inputbox.NewModel(ctx, inputbox.ModelOpts{TextArea: &localFilterArea})

	// inputbox.NewModel focuses whatever it wraps unconditionally, so blur
	// everything except the field we actually want focused.
	for i := range m.fields {
		if fieldIndex(i) != m.focus {
			m.fields[i].Blur()
		}
	}

	return m
}

func (m *Model) SetWidth(width int) {
	for i := range m.fields {
		m.fields[i].SetWidth(width)
	}
}

// Update handles one key: tab/shift+tab move focus between fields (wrapping
// around), ctrl+s and esc are reported back via Action for the caller to
// act on, and anything else is forwarded to the focused field.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, Action) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc":
			return m, nil, ActionCancel

		case "ctrl+s":
			return m, nil, ActionSubmit

		case "tab":
			m.fields[m.focus].Blur()
			m.focus = (m.focus + 1) % fieldCount
			cmd := m.fields[m.focus].Focus()
			return m, cmd, ActionNone

		case "shift+tab":
			m.fields[m.focus].Blur()
			m.focus = (m.focus - 1 + fieldCount) % fieldCount
			cmd := m.fields[m.focus].Focus()
			return m, cmd, ActionNone
		}
	}

	var cmd tea.Cmd
	m.fields[m.focus], cmd = m.fields[m.focus].Update(msg)
	return m, cmd, ActionNone
}

// Values returns the current text of every field, in the same order the
// caller needs them to build config.FieldValue entries (plus title, which
// the caller compares against the original to decide whether to rename).
func (m Model) Values() (title, filters, limit, extraFields, localFilter string) {
	return m.fields[fieldTitle].Value(),
		m.fields[fieldFilters].Value(),
		m.fields[fieldLimit].Value(),
		m.fields[fieldExtraFields].Value(),
		m.fields[fieldLocalFilter].Value()
}

func (m Model) View() string {
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(m.ctx.Theme.SecondaryText)
	focusedLabelStyle := lipgloss.NewStyle().Bold(true).Foreground(m.ctx.Theme.PrimaryText)
	helpStyle := lipgloss.NewStyle().Foreground(m.ctx.Theme.FaintText)
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.ctx.Theme.PrimaryBorder).
		Padding(1, 2)

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("Editar aba: %s", m.Title)))
	b.WriteString("\n\n")

	for i := fieldIndex(0); i < fieldCount; i++ {
		style := labelStyle
		if i == m.focus {
			style = focusedLabelStyle
		}
		b.WriteString(style.Render(fieldLabels[i]))
		b.WriteString("\n")
		b.WriteString(m.fields[i].View())
		if i < fieldCount-1 {
			b.WriteString("\n\n")
		}
	}

	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("tab/shift+tab: navegar  •  ctrl+s: salvar  •  esc: cancelar"))

	return boxStyle.Render(b.String())
}
