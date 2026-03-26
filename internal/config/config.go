package config

import "github.com/kelseyhightower/envconfig"

type Config struct {
	DatabaseURL    string `envconfig:"DATABASE_URL" required:"true"`
	KafkaBroker    string `envconfig:"KAFKA_BROKER" default:"localhost:9092"`
	ServerPort     string `envconfig:"SERVER_PORT" default:"8080"`
	MigrationsPath string `envconfig:"MIGRATIONS_PATH" default:"migrations"`
}

func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
