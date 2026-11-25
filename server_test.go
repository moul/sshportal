package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestServerDatabaseStartupErrors(t *testing.T) {
	tests := []struct {
		name   string
		driver string
		dsn    string
		want   string
	}{
		{
			name:   "unknown driver",
			driver: "unknown",
			want:   "unsupported database driver",
		},
		{
			name:   "invalid SQLite path",
			driver: "sqlite3",
			dsn:    filepath.Join("missing-directory", "sshportal.db"),
			want:   "cannot open sqlite3 database",
		},
		{
			name:   "invalid MySQL connection string",
			driver: "mysql",
			dsn:    "invalid-test-credential",
			want:   "cannot open mysql database",
		},
		{
			name:   "invalid PostgreSQL connection string",
			driver: "postgres",
			dsn:    "postgres://test:invalid-test-credential@invalid:port/sshportal",
			want:   "cannot open postgres database",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := server(&serverConfig{dbDriver: tt.driver, dbURL: tt.dsn})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("server() error = %v; want %q", err, tt.want)
			}
			if strings.Contains(err.Error(), "invalid-test-credential") {
				t.Fatal("server error disclosed a credential")
			}
		})
	}
}
