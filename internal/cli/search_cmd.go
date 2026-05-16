package cli

import "github.com/gookit/gcli/v3"

type SearchOptions struct {
	Keyword string
	Extras  []string
	Limit   int
	Sort    string
	Order   string
	JSON    bool
}

func newSearchCmd(handler CommandHandler) (*gcli.Command, func()) {
	opts := &SearchOptions{Limit: 10}
	cmd := gcli.NewCommand("search", "Search GitHub repositories")
	cmd.Help = `<info>Examples</>:
  eget search markview
  eget search markview language:rust user:inhere
  eget search --limit 5 --sort stars --order desc terminal ui
  eget search --json picoclaw user:sipeed`

	cmd.Config = func(c *gcli.Command) {
		c.StrOpt(&opts.Sort, "sort", "", "", "Search sort field: stars, updated")
		c.StrOpt(&opts.Order, "order", "", "", "Search order: desc, asc")
		c.IntOpt(&opts.Limit, "limit", "l", 10, "Limit result count")
		c.BoolOpt(&opts.JSON, "json", "j", false, "Output as JSON")
		c.AddArg("keyword", "keywords for search repositories", true)
		c.AddArg("extras", "extra search conditions, allow multiple", false, true)
	}
	cmd.Func = func(c *gcli.Command, args []string) error {
		opts.Keyword = c.Arg("keyword").String()
		opts.Extras = append(c.Arg("extras").Strings(), args...)
		if err := validateNoFlagArgs(append([]string{opts.Keyword}, opts.Extras...)); err != nil {
			return err
		}
		snapshot := *opts
		if len(opts.Extras) > 0 {
			snapshot.Extras = append([]string(nil), opts.Extras...)
		}
		return handler("search", &snapshot)
	}
	return cmd, func() {
		*opts = SearchOptions{Limit: 10}
	}
}
