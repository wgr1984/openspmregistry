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
	Auth     AuthConfig    `yaml:"auth"`
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

type AuthConfig struct {
	Name         string `yaml:"name"`
	Type         string `yaml:"type"`
	Enabled      bool   `yaml:"enabled"`
	ClientId     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	Issuer       string `yaml:"issuer"`
	GrantType    string `yaml:"grant_type"`
	Users        []User `yaml:"users"`
}

type User struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}
