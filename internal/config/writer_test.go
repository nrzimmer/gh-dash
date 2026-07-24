package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	yamlmarshaller "gopkg.in/yaml.v3"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o666))
	return path
}

func TestAppendSection_PopulatedListPreservesCommentsAndAppendsAtEnd(t *testing.T) {
	original := `prSections:
  - title: Meus PRs
    filters: is:open author:@me

issuesSections:
  - title: Atribuídas a mim
    filters: is:open assignee:@me
  - title: Últimos 7 dias
    filters: involves:@me

  # --- Abas do Project "Patient Care Team" (org 41) ---
  # comentário explicando o board
  - title: "[PCT] Epics"
    filters: project:tabiahealth/41 -is:closed
repoPaths:
  tabiahealth/*: ~/src/tabia/careos/*
`
	path := writeTempConfig(t, original)

	err := AppendSection(path, "issuesSections", IssuesSectionConfig{
		Title:   "Nova Aba",
		Filters: "is:open author:@me",
	})
	require.NoError(t, err)

	out, err := os.ReadFile(path)
	require.NoError(t, err)
	result := string(out)

	require.Contains(t, result, `# --- Abas do Project "Patient Care Team" (org 41) ---`)
	require.Contains(t, result, `# comentário explicando o board`)
	require.Contains(t, result, `- title: "[PCT] Epics"`)
	require.Contains(t, result, "repoPaths:")
	require.Contains(t, result, "  tabiahealth/*: ~/src/tabia/careos/*")

	require.Contains(t, result, "- title: Nova Aba")
	require.Contains(t, result, "    filters: is:open author:@me")

	// The new item must land inside issuesSections, before repoPaths, and
	// after the last existing issuesSections item.
	idxPCT := strings.Index(result, `[PCT] Epics`)
	idxNew := strings.Index(result, "Nova Aba")
	idxRepoPaths := strings.Index(result, "repoPaths:")
	require.Greater(t, idxNew, idxPCT)
	require.Less(t, idxNew, idxRepoPaths)

	// prSections must be untouched.
	require.Contains(t, result, "prSections:\n  - title: Meus PRs\n    filters: is:open author:@me")
}

func TestAppendSection_EmptyListCreatesBlock(t *testing.T) {
	original := `issuesSections: []
repoPaths: {}
`
	path := writeTempConfig(t, original)

	err := AppendSection(path, "issuesSections", IssuesSectionConfig{
		Title:   "Primeira Aba",
		Filters: "is:open",
	})
	require.NoError(t, err)

	out, err := os.ReadFile(path)
	require.NoError(t, err)
	result := string(out)

	require.NotContains(t, result, "issuesSections: []")
	require.Contains(t, result, "issuesSections:\n  - title: Primeira Aba\n    filters: is:open\n")
	require.Contains(t, result, "repoPaths: {}")
}

func TestAppendSection_MissingKeyAppendsNewTopLevelList(t *testing.T) {
	original := `prSections:
  - title: Meus PRs
    filters: is:open
`
	path := writeTempConfig(t, original)

	err := AppendSection(path, "issuesSections", IssuesSectionConfig{
		Title:   "Minhas Issues",
		Filters: "is:open author:@me",
	})
	require.NoError(t, err)

	out, err := os.ReadFile(path)
	require.NoError(t, err)
	result := string(out)

	require.Contains(t, result, "prSections:\n  - title: Meus PRs\n    filters: is:open\n")
	require.Contains(t, result, "issuesSections:\n  - title: Minhas Issues\n    filters: is:open author:@me\n")
}

func TestAppendSection_EscapesSpecialCharacters(t *testing.T) {
	original := "prSections:\n  - title: Meus PRs\n    filters: is:open\n"
	path := writeTempConfig(t, original)

	err := AppendSection(path, "prSections", PrsSectionConfig{
		Title:   `Title: with "quotes" and colon`,
		Filters: `author:@me "quoted phrase"`,
	})
	require.NoError(t, err)

	out, err := os.ReadFile(path)
	require.NoError(t, err)

	var parsed struct {
		PRSections []PrsSectionConfig `yaml:"prSections"`
	}
	require.NoError(t, yamlmarshaller.Unmarshal(out, &parsed))
	require.Len(t, parsed.PRSections, 2)
	require.Equal(t, `Title: with "quotes" and colon`, parsed.PRSections[1].Title)
	require.Equal(t, `author:@me "quoted phrase"`, parsed.PRSections[1].Filters)
}

func TestAppendSection_PreservesAnchorsElsewhereInFile(t *testing.T) {
	original := `issuesSections:
  - title: "[PCT] Epics"
    filters: project:tabiahealth/41 -is:closed
    extraFields: &pctStatusExtra |
      projectItems(first: 10) { nodes { project { number } } }
  - title: "[PCT] Planned"
    filters: project:tabiahealth/41 -is:closed
    extraFields: *pctStatusExtra
repoPaths:
  tabiahealth/*: ~/src/tabia/careos/*
`
	path := writeTempConfig(t, original)

	err := AppendSection(path, "issuesSections", IssuesSectionConfig{
		Title:   "Nova Aba",
		Filters: "is:open",
	})
	require.NoError(t, err)

	out, err := os.ReadFile(path)
	require.NoError(t, err)
	result := string(out)

	require.Contains(t, result, "&pctStatusExtra")
	require.Contains(t, result, "*pctStatusExtra")
	require.Contains(t, result, "- title: Nova Aba")
}

func TestUpdateSectionFilter_InlineValue(t *testing.T) {
	original := `prSections:
  - title: Meus PRs
    filters: is:open author:@me
  - title: Review
    filters: is:open -author:@me
repoPaths:
  tabiahealth/*: ~/src/tabia/careos/*
`
	path := writeTempConfig(t, original)

	err := UpdateSectionFilter(path, "prSections", "Meus PRs", "is:open assignee:@me")
	require.NoError(t, err)

	out, err := os.ReadFile(path)
	require.NoError(t, err)
	result := string(out)

	require.Contains(t, result, "  - title: Meus PRs\n    filters: is:open assignee:@me\n")
	// The other section must be untouched.
	require.Contains(t, result, "  - title: Review\n    filters: is:open -author:@me\n")
	require.Contains(t, result, "repoPaths:")
}

func TestUpdateSectionFilter_BlockScalarValueReplacedWithSingleLine(t *testing.T) {
	original := `issuesSections:
  - title: Últimos 7 dias
    filters: >-
      involves:@me
      updated:>={{ nowModify "-7d" }}

  # --- Abas do Project "Patient Care Team" (org 41) ---
  # comentário explicando o board
  - title: "[PCT] Epics"
    filters: project:tabiahealth/41 -is:closed
repoPaths:
  tabiahealth/*: ~/src/tabia/careos/*
`
	path := writeTempConfig(t, original)

	err := UpdateSectionFilter(path, "issuesSections", "Últimos 7 dias", "involves:@me updated:>=2026-01-01")
	require.NoError(t, err)

	out, err := os.ReadFile(path)
	require.NoError(t, err)
	result := string(out)

	require.Contains(t, result, "  - title: Últimos 7 dias\n    filters: involves:@me updated:>=2026-01-01\n")
	require.NotContains(t, result, `involves:@me
      updated`)

	// Everything after must survive untouched: the blank-line spacer, the
	// comment block, and the next item.
	require.Contains(t, result, `# --- Abas do Project "Patient Care Team" (org 41) ---`)
	require.Contains(t, result, "# comentário explicando o board")
	require.Contains(t, result, `- title: "[PCT] Epics"`)
	require.Contains(t, result, "    filters: project:tabiahealth/41 -is:closed")
	require.Contains(t, result, "repoPaths:")
}

func TestUpdateSectionFilter_TitleNotFoundReturnsErrorWithoutWriting(t *testing.T) {
	original := "prSections:\n  - title: Meus PRs\n    filters: is:open\n"
	path := writeTempConfig(t, original)

	err := UpdateSectionFilter(path, "prSections", "Não existe", "is:open")
	require.Error(t, err)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, string(after))
}

func TestUpdateSectionFilter_PreservesAnchorsElsewhereInFile(t *testing.T) {
	original := `issuesSections:
  - title: "[PCT] Epics"
    filters: project:tabiahealth/41 -is:closed
    extraFields: &pctStatusExtra |
      projectItems(first: 10) { nodes { project { number } } }
  - title: "[PCT] Planned"
    filters: project:tabiahealth/41 -is:closed
    extraFields: *pctStatusExtra
`
	path := writeTempConfig(t, original)

	err := UpdateSectionFilter(path, "issuesSections", "[PCT] Planned", "project:tabiahealth/41 -is:closed status:new")
	require.NoError(t, err)

	out, err := os.ReadFile(path)
	require.NoError(t, err)
	result := string(out)

	require.Contains(t, result, "&pctStatusExtra")
	require.Contains(t, result, "*pctStatusExtra")
	require.Contains(t, result, "    filters: project:tabiahealth/41 -is:closed status:new")
	// The other item's filters (and its anchor definition) must survive.
	require.Contains(t, result, "  - title: \"[PCT] Epics\"\n    filters: project:tabiahealth/41 -is:closed\n")
}

func TestUpdateSectionFilter_EscapesSpecialCharacters(t *testing.T) {
	original := "prSections:\n  - title: Meus PRs\n    filters: is:open\n"
	path := writeTempConfig(t, original)

	err := UpdateSectionFilter(path, "prSections", "Meus PRs", `author:@me "quoted phrase"`)
	require.NoError(t, err)

	out, err := os.ReadFile(path)
	require.NoError(t, err)

	var parsed struct {
		PRSections []PrsSectionConfig `yaml:"prSections"`
	}
	require.NoError(t, yamlmarshaller.Unmarshal(out, &parsed))
	require.Len(t, parsed.PRSections, 1)
	require.Equal(t, "Meus PRs", parsed.PRSections[0].Title)
	require.Equal(t, `author:@me "quoted phrase"`, parsed.PRSections[0].Filters)
}

func TestUpdateSectionFields_UpdatesMultipleFieldsAtOnce(t *testing.T) {
	original := "prSections:\n  - title: Meus PRs\n    filters: is:open\n    limit: 10\n"
	path := writeTempConfig(t, original)

	err := UpdateSectionFields(path, "prSections", "Meus PRs", []FieldValue{
		{Name: "filters", Value: "is:open assignee:@me", IsSet: true},
		{Name: "limit", Value: "25", IsSet: true},
	})
	require.NoError(t, err)

	out, err := os.ReadFile(path)
	require.NoError(t, err)

	var parsed struct {
		PRSections []PrsSectionConfig `yaml:"prSections"`
	}
	require.NoError(t, yamlmarshaller.Unmarshal(out, &parsed))
	require.Len(t, parsed.PRSections, 1)
	require.Equal(t, "is:open assignee:@me", parsed.PRSections[0].Filters)
	require.NotNil(t, parsed.PRSections[0].Limit)
	require.Equal(t, 25, *parsed.PRSections[0].Limit)
	// limit must stay an unquoted YAML integer, not a quoted string.
	require.Contains(t, string(out), "limit: 25\n")
	require.NotContains(t, string(out), `limit: "25"`)
}

func TestUpdateSectionFields_InsertsFieldsThatDidNotExist(t *testing.T) {
	original := "issuesSections:\n  - title: Mine\n    filters: is:open\n"
	path := writeTempConfig(t, original)

	err := UpdateSectionFields(path, "issuesSections", "Mine", []FieldValue{
		{Name: "extraFields", Value: "state\nlabels { nodes { name } }", IsSet: true},
		{Name: "localFilter", Value: `state == "OPEN"`, IsSet: true},
	})
	require.NoError(t, err)

	out, err := os.ReadFile(path)
	require.NoError(t, err)

	var parsed struct {
		IssuesSections []IssuesSectionConfig `yaml:"issuesSections"`
	}
	require.NoError(t, yamlmarshaller.Unmarshal(out, &parsed))
	require.Len(t, parsed.IssuesSections, 1)
	require.Equal(t, "state\nlabels { nodes { name } }", parsed.IssuesSections[0].ExtraFields)
	require.Equal(t, `state == "OPEN"`, parsed.IssuesSections[0].LocalFilter)
}

func TestUpdateSectionFields_ClearingLimitRemovesTheLine(t *testing.T) {
	original := "prSections:\n  - title: Meus PRs\n    filters: is:open\n    limit: 10\n  - title: Outra\n    filters: is:open\n"
	path := writeTempConfig(t, original)

	err := UpdateSectionFields(path, "prSections", "Meus PRs", []FieldValue{
		{Name: "limit", Value: "", IsSet: false},
	})
	require.NoError(t, err)

	out, err := os.ReadFile(path)
	require.NoError(t, err)
	result := string(out)

	require.NotContains(t, result, "limit:")
	require.Contains(t, result, "  - title: Meus PRs\n    filters: is:open\n  - title: Outra\n    filters: is:open\n")
}

func TestUpdateSectionFields_MultilineExtraFieldsPreservesAnchorsElsewhere(t *testing.T) {
	original := `issuesSections:
  - title: "[PCT] Epics"
    filters: project:tabiahealth/41 -is:closed
    extraFields: &pctStatusExtra |
      projectItems(first: 10) { nodes { project { number } } }
  - title: "[PCT] Planned"
    filters: project:tabiahealth/41 -is:closed
    extraFields: *pctStatusExtra
`
	path := writeTempConfig(t, original)

	err := UpdateSectionFields(path, "issuesSections", "[PCT] Planned", []FieldValue{
		{Name: "extraFields", Value: "status\nassignees { nodes { login } }", IsSet: true},
		{Name: "localFilter", Value: "status != nil", IsSet: true},
	})
	require.NoError(t, err)

	out, err := os.ReadFile(path)
	require.NoError(t, err)
	result := string(out)

	require.Contains(t, result, "&pctStatusExtra")
	// The other item (with the anchor definition) must be untouched.
	require.Contains(t, result, "  - title: \"[PCT] Epics\"\n    filters: project:tabiahealth/41 -is:closed\n    extraFields: &pctStatusExtra |\n      projectItems(first: 10) { nodes { project { number } } }\n")

	var parsed struct {
		IssuesSections []IssuesSectionConfig `yaml:"issuesSections"`
	}
	require.NoError(t, yamlmarshaller.Unmarshal(out, &parsed))
	require.Len(t, parsed.IssuesSections, 2)
	require.Equal(t, "status\nassignees { nodes { login } }", parsed.IssuesSections[1].ExtraFields)
	require.Equal(t, "status != nil", parsed.IssuesSections[1].LocalFilter)
	// First item's extraFields (the anchor definition) must be untouched.
	require.Equal(t, "projectItems(first: 10) { nodes { project { number } } }\n", parsed.IssuesSections[0].ExtraFields)
}

func TestRenameSection_RenamesPreservingOtherFieldsAndSiblings(t *testing.T) {
	original := `prSections:
  - title: Meus PRs
    filters: is:open author:@me
    limit: 10
  - title: Outra
    filters: is:open
`
	path := writeTempConfig(t, original)

	err := RenameSection(path, "prSections", "Meus PRs", "Minhas PRs abertas")
	require.NoError(t, err)

	out, err := os.ReadFile(path)
	require.NoError(t, err)

	var parsed struct {
		PRSections []PrsSectionConfig `yaml:"prSections"`
	}
	require.NoError(t, yamlmarshaller.Unmarshal(out, &parsed))
	require.Len(t, parsed.PRSections, 2)
	require.Equal(t, "Minhas PRs abertas", parsed.PRSections[0].Title)
	require.Equal(t, "is:open author:@me", parsed.PRSections[0].Filters)
	require.NotNil(t, parsed.PRSections[0].Limit)
	require.Equal(t, 10, *parsed.PRSections[0].Limit)
	// The sibling section must be untouched.
	require.Equal(t, "Outra", parsed.PRSections[1].Title)
	require.Equal(t, "is:open", parsed.PRSections[1].Filters)
}

func TestRenameSection_TitleNotFoundReturnsErrorWithoutWriting(t *testing.T) {
	original := "prSections:\n  - title: Meus PRs\n    filters: is:open\n"
	path := writeTempConfig(t, original)

	err := RenameSection(path, "prSections", "Não existe", "Novo nome")
	require.Error(t, err)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, string(after))
}

func TestRenameSection_ThenUpdateSectionFieldsFindsItemByNewTitle(t *testing.T) {
	original := `issuesSections:
  - title: "[PCT] Epics"
    filters: project:tabiahealth/41 -is:closed
    extraFields: &pctStatusExtra |
      projectItems(first: 10) { nodes { project { number } } }
  - title: "[PCT] Planned"
    filters: project:tabiahealth/41 -is:closed
    extraFields: *pctStatusExtra
`
	path := writeTempConfig(t, original)

	require.NoError(t, RenameSection(path, "issuesSections", "[PCT] Planned", "[PCT] Em Planejamento"))
	require.NoError(t, UpdateSectionFields(path, "issuesSections", "[PCT] Em Planejamento", []FieldValue{
		{Name: "localFilter", Value: "status != nil", IsSet: true},
	}))

	out, err := os.ReadFile(path)
	require.NoError(t, err)
	result := string(out)

	require.Contains(t, result, "&pctStatusExtra")

	var parsed struct {
		IssuesSections []IssuesSectionConfig `yaml:"issuesSections"`
	}
	require.NoError(t, yamlmarshaller.Unmarshal(out, &parsed))
	require.Len(t, parsed.IssuesSections, 2)
	require.Equal(t, "[PCT] Epics", parsed.IssuesSections[0].Title)
	require.Equal(t, "[PCT] Em Planejamento", parsed.IssuesSections[1].Title)
	require.Equal(t, "status != nil", parsed.IssuesSections[1].LocalFilter)
}
