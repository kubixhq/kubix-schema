package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DBHost        string
	DBPort        int
	DBName        string
	DBUser        string
	DBPassword    string
	DBSSLMode     string
	MigrationTool string
	ServerPort    int
	SnapshotDir   string
}

func Load() Config {
	dbPort, _ := strconv.Atoi(getEnv("DB_PORT", "5432"))
	serverPort, _ := strconv.Atoi(getEnv("SERVER_PORT", "8080"))
	return Config{
		DBHost:        getEnv("DB_HOST", "localhost"),
		DBPort:        dbPort,
		DBName:        getEnv("DB_NAME", "postgres"),
		DBUser:        getEnv("DB_USER", "postgres"),
		DBPassword:    os.Getenv("DB_PASSWORD"),
		DBSSLMode:     getEnv("DB_SSL_MODE", "disable"),
		MigrationTool: getEnv("MIGRATION_TOOL", "auto"),
		ServerPort:    serverPort,
		SnapshotDir:   getEnv("SNAPSHOT_DIR", "./snapshots"),
	}
}

// Validate returns an error for any config value that would cause a silent
// misbehavior at runtime (invalid port, unknown migration tool).
func (c Config) Validate() error {
	if c.DBPort <= 0 || c.DBPort > 65535 {
		return fmt.Errorf("DB_PORT must be a number between 1 and 65535 (got %q)", os.Getenv("DB_PORT"))
	}
	switch c.MigrationTool {
	case "auto", "flyway", "liquibase", "prisma":
	default:
		return fmt.Errorf("MIGRATION_TOOL %q is invalid; must be one of: auto, flyway, liquibase, prisma", c.MigrationTool)
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
