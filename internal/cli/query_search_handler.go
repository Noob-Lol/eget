package cli

import (
	"fmt"

	"github.com/inherelab/eget/internal/app"
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
		text, err := queryResultJSON(result)
		if err != nil {
			return err
		}
		fmt.Println(text)
		return nil
	}
	printQueryResult(result)
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
		text, err := searchResultJSON(result)
		if err != nil {
			return err
		}
		fmt.Println(text)
		return nil
	}

	printSearchResult(result)
	return nil
}
