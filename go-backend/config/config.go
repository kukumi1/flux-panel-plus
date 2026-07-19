package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DBHost         string
	DBName         string
	DBUser         string
	DBPassword     string
	JWTSecret         string
	LogDir            string
	NodeBinaryDir     string
	Port              int
	AllowedOrigins    []string
	AuditIngestSecret string

	// AuditSniffingNodeIds gates GOST handler-level TLS/HTTP sniffing per node.
	// Sniffing lets forward nodes record the real visited host (SNI/Host) instead
	// of only the next hop, at the cost of a per-connection sniff. Opt-in per node
	// via AUDIT_SNIFFING_NODE_IDS so it can be validated before a wider rollout.
	AuditSniffingNodeIds map[int64]bool
}

var Cfg *Config

// ShouldSniffNode reports whether GOST sniffing is enabled for the given node.
func (c *Config) ShouldSniffNode(nodeId int64) bool {
	if c == nil || c.AuditSniffingNodeIds == nil {
		return false
	}
	return c.AuditSniffingNodeIds[nodeId]
}

func Load() {
	Cfg = &Config{
		DBHost:         getEnv("DB_HOST", "127.0.0.1"),
		DBName:         getEnv("DB_NAME", "gost"),
		DBUser:         getEnv("DB_USER", "root"),
		DBPassword:     getEnv("DB_PASSWORD", ""),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		LogDir:         getEnv("LOG_DIR", "/app/logs"),
		NodeBinaryDir:  getEnv("NODE_BINARY_DIR", "/data/node"),
		Port:           getEnvInt("SERVER_PORT", 6365),
		AllowedOrigins: parseOrigins(os.Getenv("ALLOWED_ORIGINS")),

		AuditIngestSecret:    os.Getenv("AUDIT_INGEST_SECRET"),
		AuditSniffingNodeIds: parseNodeIdSet(os.Getenv("AUDIT_SNIFFING_NODE_IDS")),
	}
}

func parseNodeIdSet(raw string) map[int64]bool {
	ids := make(map[int64]bool)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if id, err := strconv.ParseInt(part, 10, 64); err == nil {
			ids[id] = true
		}
	}
	return ids
}

func DSN() string {
	return Cfg.DBUser + ":" + Cfg.DBPassword + "@tcp(" + Cfg.DBHost + ":3306)/" + Cfg.DBName + "?charset=utf8mb4&parseTime=False&loc=Local"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseOrigins(raw string) []string {
	if raw == "" {
		return nil
	}
	var origins []string
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			origins = append(origins, o)
		}
	}
	return origins
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
