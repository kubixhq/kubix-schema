package config

import (
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

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
