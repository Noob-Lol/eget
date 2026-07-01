package app

import (
	"context"
	"sort"
	"sync"

	"github.com/inherelab/eget/internal/install"
)

func (s UpdateService) UpdateCandidates(candidates []OutdatedItem, cli install.Options) ([]UpdateResult, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	if !isAppInstallService(s.Install) {
		cli = applyConfigNetworkOptions(cfg, cli)
	}
	if err := validateRawConcurrencyOptions(cli); err != nil {
		return nil, err
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Name < candidates[j].Name
	})

	rawBatch := cli.BatchConcurrency
	if !cli.BatchConcurrencySet && rawBatch <= 0 {
		rawBatch = 0
	}
	if err := validateConcurrencyOptions(install.Options{BatchConcurrency: rawBatch}); err != nil {
		return nil, err
	}
	batch := effectiveBatchConcurrency(rawBatch, len(candidates))
	if batch > 1 {
		return s.updateCandidatesConcurrent(candidates, cli, batch)
	}

	results := make([]UpdateResult, 0, len(candidates))
	for index, item := range candidates {
		if s.OnUpdateStart != nil {
			s.OnUpdateStart(index, len(candidates), item.Name)
		}
		result, err := s.UpdatePackage(item.Name, cli)
		if err != nil {
			return nil, err
		}
		results = append(results, UpdateResult{
			Name:   item.Name,
			Target: item.Repo,
			Result: result,
		})
	}
	return results, nil
}

func isAppInstallService(installer Installer) bool {
	switch installer.(type) {
	case Service, *Service:
		return true
	default:
		return false
	}
}

func (s UpdateService) updateCandidatesConcurrent(candidates []OutdatedItem, cli install.Options, batch int) ([]UpdateResult, error) {
	type job struct {
		index int
		item  OutdatedItem
	}
	results := make([]UpdateResult, len(candidates))
	jobs := make(chan job)
	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < batch; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for work := range jobs {
				select {
				case <-ctx.Done():
					continue
				default:
				}
				result, err := s.UpdatePackage(work.item.Name, cli)
				if err != nil {
					sendFirstError(errCh, err, cancel)
					continue
				}
				results[work.index] = UpdateResult{Name: work.item.Name, Target: work.item.Repo, Result: result}
			}
		}()
	}

sendLoop:
	for index, item := range candidates {
		select {
		case <-ctx.Done():
			break sendLoop
		case jobs <- job{index: index, item: item}:
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
