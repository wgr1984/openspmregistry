package config

type ServerRoot struct {
	Server ServerConfig `yaml:"server"`
}

type ServerConfig struct {
	Hostname string        `yaml:"hostname"`
	Port     int           `yaml:"port"`
	Certs    Certs         `yaml:"certs"`
	Repo     Repo          `yaml:"repo"`
	Publish  PublishConfig `yaml:"publish"`
}

type Certs struct {
	CertFile string `yaml:"cert"`
	KeyFile  string `yaml:"key"`
}

type PublishConfig struct {
	MaxSize int64 `yaml:"maxSize"`
}

type Repo struct {
	Path string `yaml:"path"`
	Type string `yaml:"type"`
}
