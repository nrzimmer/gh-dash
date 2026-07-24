package data

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/expr-lang/expr"
)

// localFilterSearchResponse is the generic shape of the raw GraphQL response
// used to evaluate a section's LocalFilter. Every matched node is decoded as
// a plain map[string]any, so it works for any GraphQL fragment (PullRequest,
// Issue, ...) and any set of extra fields the user configures — nothing here
// is hardcoded to a particular data shape.
type localFilterSearchResponse struct {
	Search struct {
		Nodes    []map[string]any `json:"nodes"`
		PageInfo struct {
			HasNextPage bool   `json:"hasNextPage"`
			EndCursor   string `json:"endCursor"`
		} `json:"pageInfo"`
	} `json:"search"`
}

// graphQLSearchPageMax is GitHub's hard cap on `first` for the `search`
// connection: requesting more than this in a single page is a query error,
// regardless of how high a section's `limit` is configured.
const graphQLSearchPageMax = 100

// makeLocalFilterQuery builds a raw GraphQL query that runs the same search
// used to fetch the section, but only requests `number` plus whatever extra
// fields the user asked for via ExtraFields, inside the given fragment type
// (e.g. "PullRequest" or "Issue").
func makeLocalFilterQuery(fragmentType, extraFields string) string {
	return fmt.Sprintf(`
query LocalFilterSearch($query: String!, $limit: Int!, $after: String) {
  search(type: ISSUE, first: $limit, after: $after, query: $query) {
    nodes {
      ... on %s {
        number
        %s
      }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}`, fragmentType, extraFields)
}

// atMeRe matches the "@me" token in a localFilter expression, the same way
// GitHub's own search qualifiers do - word-boundary anchored so it doesn't
// misfire on an unrelated identifier like "@meta".
var atMeRe = regexp.MustCompile(`@me\b`)

// substituteViewer replaces "@me" in expr with the Go-quoted string literal
// of viewerLogin, so e.g. `author.login == @me` becomes
// `author.login == "some-login"` before expr.Compile ever sees it. This
// works as plain text substitution (rather than injecting a variable into
// expr.Run's environment) so it can't collide with a real GraphQL field
// happening to be named "me". If viewerLogin is still unknown (the viewer
// login hasn't been fetched yet), "@me" becomes "" - a harmless comparison
// that simply never matches, instead of a compile error.
func substituteViewer(localFilterExpr, viewerLogin string) string {
	if !strings.Contains(localFilterExpr, "@me") {
		return localFilterExpr
	}
	return atMeRe.ReplaceAllString(localFilterExpr, strconv.Quote(viewerLogin))
}

// filterNumbersLocally evaluates localFilterExpr (an expr-lang/expr boolean
// expression) against every node matched by fullSearchQuery (the section's
// search query, already wrapped with is:pr/is:issue etc. by the caller),
// fetching only `number` plus extraFields. It returns the set of PR/issue
// numbers for which the expression evaluated to true.
//
// If localFilterExpr is empty, this is a no-op: nil is returned and callers
// should treat that as "don't filter, keep everything".
func filterNumbersLocally(fragmentType, fullSearchQuery string, limit int, extraFields, localFilterExpr, viewerLogin string) (map[int]bool, error) {
	if localFilterExpr == "" {
		return nil, nil
	}

	if client == nil {
		return nil, fmt.Errorf("localFilter: no GraphQL client configured")
	}

	localFilterExpr = substituteViewer(localFilterExpr, viewerLogin)

	program, err := expr.Compile(localFilterExpr, expr.AsBool())
	if err != nil {
		return nil, fmt.Errorf("localFilter: invalid expression %q: %w", localFilterExpr, err)
	}

	query := makeLocalFilterQuery(fragmentType, extraFields)
	matched := make(map[int]bool)
	fetched := 0
	var after *string

	for fetched < limit {
		pageSize := limit - fetched
		if pageSize > graphQLSearchPageMax {
			pageSize = graphQLSearchPageMax
		}

		variables := map[string]any{
			"query": fullSearchQuery,
			"limit": pageSize,
			"after": after,
		}

		var resp localFilterSearchResponse
		if err := client.Do(query, variables, &resp); err != nil {
			return nil, fmt.Errorf("localFilter: query failed: %w", err)
		}

		for _, node := range resp.Search.Nodes {
			numberF, ok := node["number"].(float64)
			if !ok {
				continue
			}
			number := int(numberF)

			out, err := expr.Run(program, node)
			if err != nil {
				return nil, fmt.Errorf("localFilter: evaluation failed: %w", err)
			}
			if keep, _ := out.(bool); keep {
				matched[number] = true
			}
		}

		fetched += len(resp.Search.Nodes)
		if !resp.Search.PageInfo.HasNextPage || len(resp.Search.Nodes) == 0 {
			break
		}
		cursor := resp.Search.PageInfo.EndCursor
		after = &cursor
	}

	return matched, nil
}
