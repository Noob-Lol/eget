package app

import (
	"context"
	"fmt"
	"sort"
	"sync"

	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/install"
	"github.com/inherelab/eget/internal/util"
)

func (s Service) InstallAllPackages(cli install.Options) ([]InstallAllResult, error) {
	if err := validateRawConcurrencyOptions(cli); err != nil {
		return nil, err
	}
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	if len(cfg.Packages) == 0 {
		return nil, fmt.Errorf("no managed packages configured")
	}

	names := make([]string, 0, len(cfg.Packages))
	for name := range cfg.Packages {
		names = append(names, name)
	}
	sort.Strings(names)

	rawBatch := batchConcurrencyFromConfig(cfg, cli)
	if err := validateConcurrencyOptions(install.Options{BatchConcurrency: rawBatch}); err != nil {
		return nil, err
	}
	batch := effectiveBatchConcurrency(rawBatch, len(names))
	if batch > 1 {
		return s.installAllPackagesConcurrent(cfg, names, cli, batch)
	}

	results := make([]InstallAllResult, 0, len(names))
	for _, name := range names {
		pkg := cfg.Packages[name]
		repo := util.DerefString(pkg.Repo)
		if repo == "" {
			return nil, fmt.Errorf("package %q has no repo", name)
		}
		runTarget, recordTarget, opts, err := s.resolveInstallRequestWithConfig(cfg, name, cli, false)
		if err != nil {
			return nil, err
		}
		if err := validateConcurrencyOptions(opts); err != nil {
			return nil, err
		}
		opts = applyDefaultInstallTarget(opts)
		opts = normalizeExtractionOptions(opts)
		result, err := s.installResolvedTarget(runTarget, recordTarget, opts)
		if err != nil {
			return nil, err
		}
		results = append(results, InstallAllResult{
			Name:   name,
			Target: runTarget,
			Result: result,
		})
	}
	return results, nil
}

func (s Service) installAllPackagesConcurrent(cfg *cfgpkg.File, names []string, cli install.Options, batch int) ([]InstallAllResult, error) {
	type job struct {
		index int
		name  string
	}
	results := make([]InstallAllResult, len(names))
	jobs := make(chan job)
	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < batch; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				select {
				case <-ctx.Done():
					continue
				default:
				}
				runTarget, recordTarget, opts, err := s.resolveInstallRequestWithConfig(cfg, item.name, cli, false)
				if err != nil {
					sendFirstError(errCh, err, cancel)
					continue
				}
				if err := validateConcurrencyOptions(opts); err != nil {
					sendFirstError(errCh, err, cancel)
					continue
				}
				opts = applyDefaultInstallTarget(opts)
				opts = normalizeExtractionOptions(opts)
				result, err := s.installResolvedTarget(runTarget, recordTarget, opts)
				if err != nil {
					sendFirstError(errCh, err, cancel)
					continue
				}
				results[item.index] = InstallAllResult{Name: item.name, Target: runTarget, Result: result}
			}
		}()
	}

sendLoop:
	for index, name := range names {
		select {
		case <-ctx.Done():
			break sendLoop
		case jobs <- job{index: index, name: name}:
		}
	}
	close(jobs)
	wg.Wait()

	select {
	case err := <-errCh:
		return nil, err
	default:
	}
	return results, nil
}
