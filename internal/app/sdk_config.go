package app

import (
	"fmt"
	"strings"

	cfgpkg "github.com/inherelab/eget/internal/config"
	"github.com/inherelab/eget/internal/sdk"
)

const (
	SDKConfigActionAdded   = "added"
	SDKConfigActionUpdated = "updated"
	SDKConfigActionSkipped = "skipped"
)

type SDKConfigAddOptions struct {
	Name   string
	All    bool
	Force  bool
	Mirror string
}

type SDKConfigAddResult struct {
	Items []SDKConfigAddItem
}

type SDKConfigAddItem struct {
	Name   string
	Action string
	Reason string
	Source string
}

func (s ConfigService) AddSDKConfig(opts SDKConfigAddOptions) (SDKConfigAddResult, error) {
	if (strings.TrimSpace(opts.Name) == "") == !opts.All {
		return SDKConfigAddResult{}, fmt.Errorf("sdk config add requires exactly one of <name> or --all")
	}
	source := sdk.BuiltinConfigOfficial
	if strings.TrimSpace(opts.Mirror) != "" {
		source = sdk.BuiltinConfigSource(strings.TrimSpace(opts.Mirror))
	}

	cfg, err := s.load()
	if err != nil {
		return SDKConfigAddResult{}, err
	}
	if cfg.SDK == nil {
		cfg.SDK = make(map[string]cfgpkg.SDKSection)
	}

	var builtins []sdk.BuiltinConfig
	if opts.All {
		builtins = sdk.BuiltinConfigs(source)
	} else {
		builtin, ok := sdk.FindBuiltinConfig(opts.Name, source)
		if !ok {
			if source != sdk.BuiltinConfigOfficial {
				return SDKConfigAddResult{}, fmt.Errorf("unknown built-in SDK config %q for mirror %q; available mirrors: %s", opts.Name, source, strings.Join(sdk.BuiltinMirrorNames(), ", "))
			}
			return SDKConfigAddResult{}, fmt.Errorf("unknown built-in SDK config %q; available: %s", opts.Name, strings.Join(sdk.BuiltinConfigNames(), ", "))
		}
		builtins = []sdk.BuiltinConfig{builtin}
	}
	if len(builtins) == 0 {
		return SDKConfigAddResult{}, fmt.Errorf("unknown built-in SDK config source %q; available mirrors: %s", source, strings.Join(sdk.BuiltinMirrorNames(), ", "))
	}

	result := SDKConfigAddResult{Items: make([]SDKConfigAddItem, 0, len(builtins))}
	changed := false
	for _, builtin := range builtins {
		if _, exists := cfg.SDK[builtin.Name]; exists && !opts.Force {
			if !opts.All {
				return SDKConfigAddResult{}, fmt.Errorf("sdk config %s already exists, use --force to overwrite", builtin.Name)
			}
			result.Items = append(result.Items, SDKConfigAddItem{Name: builtin.Name, Action: SDKConfigActionSkipped, Reason: "already exists", Source: string(source)})
			continue
		}
		action := SDKConfigActionAdded
		if _, exists := cfg.SDK[builtin.Name]; exists {
			action = SDKConfigActionUpdated
		}
		cfg.SDK[builtin.Name] = builtin.Section
		result.Items = append(result.Items, SDKConfigAddItem{Name: builtin.Name, Action: action, Source: string(source)})
		changed = true
	}
	if changed {
		if err := s.save(cfg); err != nil {
			return SDKConfigAddResult{}, err
		}
	}
	return result, nil
}
