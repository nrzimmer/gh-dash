package data

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubstituteViewer_ReplacesAtMeWithQuotedLogin(t *testing.T) {
	result := substituteViewer(`author.login == @me`, "nrzimmer")
	require.Equal(t, `author.login == "nrzimmer"`, result)
}

func TestSubstituteViewer_ReplacesMultipleOccurrences(t *testing.T) {
	result := substituteViewer(
		`author.login == @me or any(assignees.nodes, {.login == @me})`,
		"nrzimmer",
	)
	require.Equal(t,
		`author.login == "nrzimmer" or any(assignees.nodes, {.login == "nrzimmer"})`,
		result,
	)
}

func TestSubstituteViewer_LeavesExpressionsWithoutAtMeUnchanged(t *testing.T) {
	expr := `mergeable == "CONFLICTING"`
	require.Equal(t, expr, substituteViewer(expr, "nrzimmer"))
}

func TestSubstituteViewer_DoesNotMatchLongerIdentifierPrefix(t *testing.T) {
	// "@meta" must not be treated as "@me" + "ta".
	expr := `author.login == "@metabot"`
	require.Equal(t, expr, substituteViewer(expr, "nrzimmer"))
}

func TestSubstituteViewer_EmptyViewerLoginBecomesEmptyStringLiteral(t *testing.T) {
	result := substituteViewer(`author.login == @me`, "")
	require.Equal(t, `author.login == ""`, result)
}

func TestSubstituteViewer_EscapesQuotesInLogin(t *testing.T) {
	// Logins can't actually contain quotes, but the substitution must still
	// produce a valid, safely-escaped Go/expr string literal regardless.
	result := substituteViewer(`x == @me`, `weird"login`)
	require.Equal(t, `x == "weird\"login"`, result)
}
