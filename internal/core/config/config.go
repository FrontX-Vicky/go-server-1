package config

import (
	"fmt"
	"os"
	"strings"
)

type ServerConfig struct {
	Addr     string
	Port     string
	BasePath string
}

type Observability struct {
	Pprof      bool
	Prometheus bool
}

type PrismConfig struct {
	BaseURL   string
	TimeoutMS int
	APIKey    string
}

type Config struct {
	AppEnv  string
	Server  ServerConfig
	Obs     Observability
	APIKeys APIKeys
	Prism   PrismConfig
}

type DBConfig struct {
	Host   string
	Port   string
	User   string
	Pass   string
	Name   string
	Params string
}

func (d DBConfig) DSN() string {
	if d.Host == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?%s",
		d.User, d.Pass, d.Host, d.Port, d.Name, d.Params)
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		var parsed int
		_, err := fmt.Sscanf(v, "%d", &parsed)
		if err == nil && parsed > 0 {
			return parsed
		}
	}
	return def
}

func Load() Config {
	return Config{
		AppEnv: getenv("APP_ENV", "dev"),
		Server: ServerConfig{
			Addr:     getenv("SERVER_ADDR", "127.0.0.1"),
			Port:     getenv("SERVER_PORT", "5000"),
			BasePath: getenv("BASE_PATH", "/"),
		},
		Obs: Observability{
			Pprof:      getenv("PPROF_ENABLED", "false") == "true",
			Prometheus: getenv("PROMETHEUS_ENABLED", "false") == "true",
		},
		APIKeys: APIKeys{
			Dynamic: getenv("DYNAMIC_API_KEY", ""),
			Open:    getenv("OPEN_API_KEY", ""),
		},
		Prism: PrismConfig{
			BaseURL:   getenv("PRISM_API_BASE_URL", ""),
			TimeoutMS: getenvInt("PRISM_API_TIMEOUT_MS", 5000),
			APIKey:    getenv("PRISM_API_KEY", ""),
		},
	}
}

type APIKeys struct {
	Dynamic string
	Open    string
}

// DB_NAMES=DB1,DB2
func DBNames() []string {
	raw := getenv("DB_NAMES", "")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Reads <PFX>_HOST/_PORT/_USER/_PASS/_NAME/_PARAMS
func DBConfigFromPrefix(prefix string) DBConfig {
	p := strings.ToUpper(strings.TrimSpace(prefix))
	get := func(suffix, def string) string { return getenv(p+"_"+suffix, def) }
	return DBConfig{
		Host:   get("HOST", ""),
		Port:   get("PORT", "3306"),
		User:   get("USER", ""),
		Pass:   get("PASS", ""),
		Name:   get("NAME", ""),
		Params: get("PARAMS", "charset=utf8mb4&parseTime=True&loc=Local&checkConnLiveness=true&interpolateParams=true&timeout=30s&readTimeout=60s&writeTimeout=60s"),
	}
}
