package cli

import (
	"io"
	"os"

	"github.com/inherelab/eget/internal/app"
)

type cliService struct {
	appService       app.Service
	cfgService       app.ConfigService
	listService      app.ListService
	queryService     app.QueryService
	searchService    app.SearchService
	uninstallService app.UninstallService
	updService       app.UpdateService

	stderr             io.Writer
	configPathResolver func() (string, error)
	lookupEnv          func(string) (string, bool)
	lookupUserHome     func(string) (string, error)
	fileExists         func(string) bool
	proxyURL           string
}

func (s *cliService) stderrWriter() io.Writer {
	if s != nil && s.stderr != nil {
		return s.stderr
	}
	return os.Stderr
}
