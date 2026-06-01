package cli

import (
	"fmt"

	"github.com/inherelab/eget/internal/app"
	clirender "github.com/inherelab/eget/internal/cli/render"
)

func (s *cliService) handleQuery(opts *QueryOptions) error {
	result, err := s.queryService.Query(app.QueryOptions{
		Repo:       opts.Target,
		Action:     opts.Action,
		Tag:        opts.Tag,
		Limit:      opts.Limit,
		JSON:       opts.JSON,
		Prerelease: opts.Prerelease,
	})
	if err != nil {
		return err
	}
	if opts.JSON {
		text, err := clirender.QueryResultJSON(result)
		if err != nil {
			return err
		}
		fmt.Println(text)
		return nil
	}
	clirender.PrintQueryResult(result)
	return nil
}

func (s *cliService) handleSearch(opts *SearchOptions) error {
	result, err := s.searchService.Search(app.SearchOptions{
		Keyword: opts.Keyword,
		Extras:  opts.Extras,
		Limit:   opts.Limit,
		Sort:    opts.Sort,
		Order:   opts.Order,
	})
	if err != nil {
		return err
	}

	if opts.JSON {
		text, err := clirender.SearchResultJSON(result)
		if err != nil {
			return err
		}
		fmt.Println(text)
		return nil
	}

	clirender.PrintSearchResult(result)
	return nil
}
