package config

type ServerRoot struct {
	Server ServerConfig `yaml:"server"`
}

type ServerConfig struct {
	Port    int           `yaml:"port"`
	Publish PublishConfig `yaml:"publish"`
}

type PublishConfig struct {
	MaxSize int64 `yaml:"maxSize"`
}
