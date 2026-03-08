package config

// ContextKey is a custom type for context keys to avoid collisions (SA1029).
type ContextKey string

type ServerRoot struct {
	Server ServerConfig `yaml:"server"`
}

type ServerConfig struct {
	Hostname           string                   `yaml:"hostname"`
	Port               int                      `yaml:"port"`
	Certs              Certs                    `yaml:"certs"`
	Repo               Repo                     `yaml:"repo"`
	ListPageSize       int                      `yaml:"listPageSize"` // When >0, list endpoint paginates with ?page=N (spec 4.1). Default 10.
	Publish            PublishConfig            `yaml:"publish"`
	Auth               AuthConfig               `yaml:"auth"`
	TlsEnabled         bool                     `yaml:"tlsEnabled"`
	PackageCollections PackageCollectionsConfig `yaml:"packageCollections"`
}

type Certs struct {
	CertFile string `yaml:"cert"`
	KeyFile  string `yaml:"key"`
}

type PublishConfig struct {
	MaxSize int64 `yaml:"maxSize"`
}

type Repo struct {
	Path  string      `yaml:"path"`
	Type  string      `yaml:"type"`
	Maven MavenConfig `yaml:"maven"`
}

type MavenConfig struct {
	BaseURL       string `yaml:"baseURL"`
	GroupIdPrefix string `yaml:"groupIdPrefix"`
	AuthMode      string `yaml:"authMode"`
	Username      string `yaml:"username"`
	Password      string `yaml:"password"`
	Timeout       int    `yaml:"timeout"`
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

type PackageCollectionsConfig struct {
	Enabled            bool `yaml:"enabled"`
	RequirePackageJson bool `yaml:"requirePackageJson"`
	// PublicRead allows unauthenticated GET /collection and /collection/{scope}.
	// Required for swift package-collection add, which fetches the URL without credentials.
	PublicRead bool `yaml:"publicRead"`
	// AllowAuthQueryParam allows ?auth=<base64(Authorization)> on collection paths only, for clients
	// that cannot send headers (e.g. swift package-collection add). Off by default to avoid credential
	// leakage via logs, referrers, and proxies. When true, decoded value must start with "Basic " or "Bearer ".
	AllowAuthQueryParam bool `yaml:"allowAuthQueryParam"`
}

const (
	// AuthHeaderContextKey is the context key for the Authorization header (passthrough auth).
	AuthHeaderContextKey ContextKey = "Authorization"
)
