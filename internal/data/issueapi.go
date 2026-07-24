package data

import (
	"fmt"
	"net/url"
	"time"

	"charm.land/log/v2"
	gh "github.com/cli/go-gh/v2/pkg/api"
	graphql "github.com/cli/shurcooL-graphql"
	"github.com/shurcooL/githubv4"

	"github.com/dlvhdr/gh-dash/v4/internal/tui/theme"
)

type IssueData struct {
	Number int
	Title  string
	Body   string
	State  string
	Author struct {
		Login string
	}
	AuthorAssociation string
	UpdatedAt         time.Time
	CreatedAt         time.Time
	Url               string
	Repository        Repository
	Assignees         Assignees      `graphql:"assignees(first: 3)"`
	Comments          IssueComments  `graphql:"comments(last: 15)"`
	Reactions         IssueReactions `graphql:"reactions(first: 1)"`
	Labels            IssueLabels    `graphql:"labels(first: 20)"`
}

type IssueComments struct {
	Nodes      []IssueComment
	TotalCount int
}

type IssueComment struct {
	Author struct {
		Login string
	}
	Body      string
	UpdatedAt time.Time
}

type IssueReactions struct {
	TotalCount int
}

type Label struct {
	Color       string
	Name        string
	Description string
}

type IssueLabels struct {
	Nodes []Label
}

func (data IssueData) GetAuthor(theme theme.Theme, showAuthorIcons bool) string {
	author := data.Author.Login
	if showAuthorIcons {
		author += fmt.Sprintf(" %s", GetAuthorRoleIcon(data.AuthorAssociation, theme))
	}
	return author
}

func (data IssueData) GetTitle() string {
	return data.Title
}

func (data IssueData) GetRepoNameWithOwner() string {
	return data.Repository.NameWithOwner
}

func (data IssueData) GetRepoNameAndOwner() (owner, repoName string) {
	return data.Repository.Owner.Login, data.Repository.Name
}

func (data IssueData) GetNumber() int {
	return data.Number
}

func (data IssueData) GetUrl() string {
	return data.Url
}

func (data IssueData) GetUpdatedAt() time.Time {
	return data.UpdatedAt
}

func (data IssueData) GetCreatedAt() time.Time {
	return data.CreatedAt
}

func makeIssuesQuery(query string) string {
	return fmt.Sprintf("is:issue archived:false %s sort:updated", query)
}

func FetchIssues(query string, limit int, pageInfo *PageInfo) (IssuesResponse, error) {
	return FetchIssuesLocalFiltered(query, limit, pageInfo, "", "", "")
}

// FetchIssuesLocalFiltered behaves like FetchIssues, but when localFilter is
// non-empty it additionally runs a second, unpaginated raw GraphQL query
// (see filterNumbersLocally) requesting extraFields and drops any issue for
// which the expression evaluates to false. viewerLogin (the logged-in
// user's login) is substituted for any "@me" in localFilter.
func FetchIssuesLocalFiltered(query string, limit int, pageInfo *PageInfo, extraFields, localFilter, viewerLogin string) (IssuesResponse, error) {
	var err error
	if client == nil {
		client, err = gh.DefaultGraphQLClient()
	}

	if err != nil {
		return IssuesResponse{}, err
	}

	// GitHub caps `first` on the search connection at 100 per page, so a
	// section `limit` above that (e.g. to give localFilter enough items to
	// scan) is fetched here as multiple pages, merged into one response.
	var endCursor *string
	if pageInfo != nil {
		endCursor = &pageInfo.EndCursor
	}

	issues := make([]IssueData, 0, limit)
	var lastPageInfo PageInfo
	var issueCount int
	remaining := limit
	for remaining > 0 {
		pageSize := remaining
		if pageSize > graphQLSearchPageMax {
			pageSize = graphQLSearchPageMax
		}

		var queryResult struct {
			Search struct {
				Nodes []struct {
					Issue IssueData `graphql:"... on Issue"`
				}
				IssueCount int
				PageInfo   PageInfo
			} `graphql:"search(type: ISSUE, first: $limit, after: $endCursor, query: $query)"`
		}
		variables := map[string]any{
			"query":     graphql.String(makeIssuesQuery(query)),
			"limit":     graphql.Int(pageSize),
			"endCursor": (*graphql.String)(endCursor),
		}
		log.Debug("Fetching issues", "query", query, "limit", pageSize, "endCursor", endCursor)
		if err := client.Query("SearchIssues", &queryResult, variables); err != nil {
			return IssuesResponse{}, err
		}

		issueCount = queryResult.Search.IssueCount
		for _, node := range queryResult.Search.Nodes {
			issues = append(issues, node.Issue)
		}
		lastPageInfo = queryResult.Search.PageInfo
		remaining -= len(queryResult.Search.Nodes)

		if !queryResult.Search.PageInfo.HasNextPage || len(queryResult.Search.Nodes) == 0 {
			break
		}
		cursor := queryResult.Search.PageInfo.EndCursor
		endCursor = &cursor
	}
	log.Info("Successfully fetched issues", "query", query, "count", issueCount)

	if localFilter != "" {
		matched, err := filterNumbersLocally("Issue", makeIssuesQuery(query), limit, extraFields, localFilter, viewerLogin)
		if err != nil {
			return IssuesResponse{}, err
		}
		filtered := make([]IssueData, 0, len(issues))
		for _, issue := range issues {
			if matched[issue.Number] {
				filtered = append(filtered, issue)
			}
		}
		issues = filtered
	}

	totalCount := issueCount
	if localFilter != "" {
		totalCount = len(issues)
	}

	return IssuesResponse{
		Issues:     issues,
		TotalCount: totalCount,
		PageInfo:   lastPageInfo,
	}, nil
}

type IssuesResponse struct {
	Issues     []IssueData
	TotalCount int
	PageInfo   PageInfo
}

// FetchIssue fetches a single issue by its GitHub URL
func FetchIssue(issueUrl string) (IssueData, error) {
	var err error
	if client == nil {
		client, err = gh.DefaultGraphQLClient()
		if err != nil {
			return IssueData{}, err
		}
	}

	var queryResult struct {
		Resource struct {
			Issue IssueData `graphql:"... on Issue"`
		} `graphql:"resource(url: $url)"`
	}
	parsedUrl, err := url.Parse(issueUrl)
	if err != nil {
		return IssueData{}, err
	}
	variables := map[string]any{
		"url": githubv4.URI{URL: parsedUrl},
	}
	log.Debug("Fetching Issue", "url", issueUrl)
	err = client.Query("FetchIssue", &queryResult, variables)
	if err != nil {
		return IssueData{}, err
	}
	log.Info("Successfully fetched Issue", "url", issueUrl)

	return queryResult.Resource.Issue, nil
}
