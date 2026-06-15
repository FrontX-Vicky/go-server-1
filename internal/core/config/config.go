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

// EmailSenderAccount holds SMTP credentials for a single named sender.
type EmailSenderAccount struct {
	SMTPHost  string
	SMTPPort  string
	SMTPUser  string
	SMTPPass  string
	FromEmail string
	FromName  string
}

type EmailConfig struct {
	SMTPHost      string
	SMTPPort      string
	SMTPUser      string
	SMTPPass      string
	FromEmail     string
	FromName      string
	AttachmentDir string
	WorkerPollMS  int
	MaxAttempts   int
	// Senders holds named additional sender accounts loaded from EMAIL_SENDERS env.
	// Key is the sender key (e.g. "support", "billing"). The default sender
	// (from the EMAIL_* vars above) is always available under the key "default".
	Senders map[string]EmailSenderAccount
}

type Config struct {
	AppEnv  string
	Server  ServerConfig
	Obs     Observability
	APIKeys APIKeys
	Prism   PrismConfig
	Email   EmailConfig
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
		Email: EmailConfig{
			SMTPHost:      getenv("EMAIL_SMTP_HOST", ""),
			SMTPPort:      getenv("EMAIL_SMTP_PORT", "587"),
			SMTPUser:      getenv("EMAIL_SMTP_USER", ""),
			SMTPPass:      getenv("EMAIL_SMTP_PASS", ""),
			FromEmail:     getenv("EMAIL_FROM_EMAIL", ""),
			FromName:      getenv("EMAIL_FROM_NAME", ""),
			AttachmentDir: getenv("EMAIL_ATTACHMENT_DIR", "/tmp/markx-email-attachments"),
			WorkerPollMS:  getenvInt("EMAIL_WORKER_POLL_MS", 5000),
			MaxAttempts:   getenvInt("EMAIL_MAX_ATTEMPTS", 10),
			Senders:       loadEmailSenders(),
		},
	}
}

// loadEmailSenders reads EMAIL_SENDERS (comma-separated keys) and builds a map
// of sender accounts from EMAIL_SENDER_{KEY}_* env vars.
// Example: EMAIL_SENDERS=support,billing
//
//	EMAIL_SENDER_SUPPORT_FROM_EMAIL=support@example.com
//	EMAIL_SENDER_SUPPORT_FROM_NAME=Support Team
//	EMAIL_SENDER_SUPPORT_SMTP_HOST=smtp.gmail.com
//	EMAIL_SENDER_SUPPORT_SMTP_PORT=587
//	EMAIL_SENDER_SUPPORT_SMTP_USER=support@example.com
//	EMAIL_SENDER_SUPPORT_SMTP_PASS=app-password
func loadEmailSenders() map[string]EmailSenderAccount {
	raw := getenv("EMAIL_SENDERS", "")
	senders := make(map[string]EmailSenderAccount)
	if strings.TrimSpace(raw) == "" {
		return senders
	}
	for _, key := range strings.Split(raw, ",") {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		prefix := "EMAIL_SENDER_" + strings.ToUpper(key)
		senders[key] = EmailSenderAccount{
			SMTPHost:  getenv(prefix+"_SMTP_HOST", ""),
			SMTPPort:  getenv(prefix+"_SMTP_PORT", "587"),
			SMTPUser:  getenv(prefix+"_SMTP_USER", ""),
			SMTPPass:  getenv(prefix+"_SMTP_PASS", ""),
			FromEmail: getenv(prefix+"_FROM_EMAIL", ""),
			FromName:  getenv(prefix+"_FROM_NAME", ""),
		}
	}
	return senders
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
