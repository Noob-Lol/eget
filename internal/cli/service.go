package cli

import (
	"context"
	"io"
	"os"

	"github.com/inherelab/eget/internal/app"
	"github.com/inherelab/eget/internal/sdk"
)

type sdkCLIService interface {
	InstallMany(context.Context, []string, sdk.InstallOptions) ([]sdk.InstallResult, error)
	List(string) ([]sdk.InstalledEntry, error)
	Remove(string) (sdk.RemoveResult, error)
	RefreshIndex(context.Context, string) (sdk.Index, error)
	RefreshAllIndexes(context.Context) ([]sdk.Index, error)
	ShowIndex(string) (sdk.Index, error)
	ListIndexes() ([]sdk.CachedIndexInfo, error)
	ClearIndex(string) error
	ClearAllIndexes() error
}

type cliService struct {
	appService       app.Service
	cfgService       app.ConfigService
	listService      app.ListService
	queryService     app.QueryService
	searchService    app.SearchService
	uninstallService app.UninstallService
	updService       app.UpdateService
	sdkService       sdkCLIService

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
