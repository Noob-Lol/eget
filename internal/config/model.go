package config

type Section struct {
	ExtractAll           *bool             `toml:"extract_all" mapstructure:"extract_all"`
	AssetFilters         []string          `toml:"asset_filters" mapstructure:"asset_filters"`
	CacheDir             *string           `toml:"cache_dir" mapstructure:"cache_dir"`
	ProxyURL             *string           `toml:"proxy_url" mapstructure:"proxy_url"`
	DownloadOnly         *bool             `toml:"download_only" mapstructure:"download_only"`
	Desc                 *string           `toml:"desc" mapstructure:"desc"`
	File                 *string           `toml:"file" mapstructure:"file"`
	GithubToken          *string           `toml:"github_token" mapstructure:"github_token"`
	GuiTarget            *string           `toml:"gui_target" mapstructure:"gui_target"`
	IgnoreUpdatePackages []string          `toml:"ignore_update_packages" mapstructure:"ignore_update_packages"`
	IsGUI                *bool             `toml:"is_gui" mapstructure:"is_gui"`
	Name                 *string           `toml:"name" mapstructure:"name"`
	Quiet                *bool             `toml:"quiet" mapstructure:"quiet"`
	Repo                 *string           `toml:"repo" mapstructure:"repo"`
	ShowHash             *bool             `toml:"show_hash" mapstructure:"show_hash"`
	Source               *bool             `toml:"download_source" mapstructure:"download_source"`
	SourcePath           *string           `toml:"source_path" mapstructure:"source_path"`
	Sys7zPath            *string           `toml:"sys7z_path" mapstructure:"sys7z_path"`
	SDKTarget            *string           `toml:"sdk_target" mapstructure:"sdk_target"`
	SDKExtMap            map[string]string `toml:"sdk_ext_map" mapstructure:"sdk_ext_map"`
	System               *string           `toml:"system" mapstructure:"system"`
	Tag                  *string           `toml:"tag" mapstructure:"tag"`
	Target               *string           `toml:"target" mapstructure:"target"`
	UpgradeOnly          *bool             `toml:"upgrade_only" mapstructure:"upgrade_only"`
	Verify               *string           `toml:"verify_sha256" mapstructure:"verify_sha256"`
	DisableSSL           *bool             `toml:"disable_ssl" mapstructure:"disable_ssl"`
	ChunkConcurrency     *int              `toml:"chunk_concurrency" mapstructure:"chunk_concurrency"`
	BatchConcurrency     *int              `toml:"batch_concurrency" mapstructure:"batch_concurrency"`
}

type SDKSection struct {
	Aliases         []string          `toml:"aliases" mapstructure:"aliases"`
	Target          *string           `toml:"target" mapstructure:"target"`
	URLTemplate     *string           `toml:"url_template" mapstructure:"url_template"`
	IndexURL        *string           `toml:"index_url" mapstructure:"index_url"`
	IndexFormat     *string           `toml:"index_format" mapstructure:"index_format"`
	IndexParser     *string           `toml:"index_parser" mapstructure:"index_parser"`
	IndexPathPrefix *string           `toml:"index_path_prefix" mapstructure:"index_path_prefix"`
	FilenamePattern *string           `toml:"filename_pattern" mapstructure:"filename_pattern"`
	StripComponents *int              `toml:"strip_components" mapstructure:"strip_components"`
	OSMap           map[string]string `toml:"os_map" mapstructure:"os_map"`
	ArchMap         map[string]string `toml:"arch_map" mapstructure:"arch_map"`
	ExtMap          map[string]string `toml:"ext_map" mapstructure:"ext_map"`
}

type APICacheSection struct {
	Enable    *bool `toml:"enable" mapstructure:"enable"`
	CacheTime *int  `toml:"cache_time" mapstructure:"cache_time"`
}

type GhproxySection struct {
	Enable     *bool    `toml:"enable" mapstructure:"enable"`
	HostURL    *string  `toml:"host_url" mapstructure:"host_url"`
	SupportAPI *bool    `toml:"support_api" mapstructure:"support_api"`
	Fallbacks  []string `toml:"fallbacks" mapstructure:"fallbacks"`
}

type File struct {
	Meta struct {
		Keys []string
	}
	Global   Section         `toml:"global" mapstructure:"global"`
	ApiCache APICacheSection `toml:"api_cache" mapstructure:"api_cache"`
	Ghproxy  GhproxySection  `toml:"ghproxy" mapstructure:"ghproxy"`
	Repos    map[string]Section
	Packages map[string]Section    `toml:"packages" mapstructure:"packages"`
	SDK      map[string]SDKSection `toml:"sdk" mapstructure:"sdk"`
}

type Merged struct {
	ExtractAll       bool
	AssetFilters     []string
	CacheDir         string
	ProxyURL         string
	DownloadOnly     bool
	File             string
	GithubToken      string
	GuiTarget        string
	IsGUI            bool
	Name             string
	Quiet            bool
	ShowHash         bool
	Source           bool
	SourcePath       string
	Sys7zPath        string
	System           string
	Tag              string
	Target           string
	UpgradeOnly      bool
	Verify           string
	DisableSSL       bool
	ChunkConcurrency int
}

type CLIOverrides struct {
	ExtractAll       *bool
	AssetFilters     *[]string
	CacheDir         *string
	ProxyURL         *string
	DownloadOnly     *bool
	File             *string
	GithubToken      *string
	IsGUI            *bool
	Name             *string
	Quiet            *bool
	ShowHash         *bool
	Source           *bool
	SourcePath       *string
	System           *string
	Tag              *string
	Target           *string
	UpgradeOnly      *bool
	Verify           *string
	DisableSSL       *bool
	ChunkConcurrency *int
}
