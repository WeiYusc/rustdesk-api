package config

type Gin struct {
	ApiAddr       string `mapstructure:"api-addr"`
	AdminAddr     string `mapstructure:"admin-addr"`
	Mode          string
	ResourcesPath string `mapstructure:"resources-path"`
	TrustProxy    string `mapstructure:"trust-proxy"`
	BodyMaxSizeMb int64  `mapstructure:"body-max-size-mb"`
}

func (g *Gin) Init() {
	if g.BodyMaxSizeMb <= 0 {
		g.BodyMaxSizeMb = 10
	}
}
