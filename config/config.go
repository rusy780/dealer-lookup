package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	InputPath        string
	OutputDir        string
	CachePath        string
	CacheWarmupPaths string
	MaxLookupsPerRun int
	BatchSize        int
	CacheTTL         time.Duration

	// Local OpenClaw / OpenAI-compatible settings.
	OpenClawEnabled bool
	OpenClawURL     string
	OpenClawModel   string
	Concurrency     int
	Temperature     float64
	MaxTokens       int
	TokenPrice      float64
	UseMockIfDown   bool
	OpenClawTimeout time.Duration
}

func Load() Config {
	return Config{
		InputPath:        env("INPUT_CSV", "data/input/inventory.csv"),
		OutputDir:        env("OUTPUT_DIR", "data/output"),
		CachePath:        env("CACHE_PATH", "data/cache.json"),
		CacheWarmupPaths: env("CACHE_WARMUP_PATHS", ""),
		MaxLookupsPerRun: envInt("MAX_LOOKUPS_PER_RUN", 0),
		BatchSize:        envInt("BATCH_SIZE", 250),
		CacheTTL:         time.Duration(envInt("CACHE_TTL_DAYS", 30)) * 24 * time.Hour,

		OpenClawEnabled: envBool("OPENCLAW_ENABLED", true),
		OpenClawURL: firstNonEmpty(
			os.Getenv("OPENCLAW_BASE_URL"),
			os.Getenv("OPENCLAW_URL"),
			os.Getenv("OPENCLAW_API_URL"),
			"cli",
		),
		OpenClawModel:   env("OPENCLAW_MODEL", ""),
		Concurrency:     envInt("OPENCLAW_CONCURRENCY", 2),
		Temperature:     envFloat("OPENCLAW_TEMPERATURE", 0.1),
		MaxTokens:       envInt("OPENCLAW_MAX_TOKENS", 4000),
		TokenPrice:      envFloat("TOKEN_PRICE_PER_MILLION", 0.05),
		UseMockIfDown:   envBool("USE_MOCK_IF_DOWN", false),
		OpenClawTimeout: time.Duration(envInt("OPENCLAW_TIMEOUT_SECONDS", 120)) * time.Second,
	}
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func envInt(k string, def int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envFloat(k string, def float64) float64 {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return n
}

func envBool(k string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
