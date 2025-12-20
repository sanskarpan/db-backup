package validation

import (
	"testing"
)

func TestValidateDatabaseName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "mydb", false},
		{"valid with underscore", "my_database", false},
		{"valid with hyphen", "my-database", false},
		{"valid with numbers", "db123", false},
		{"empty", "", true},
		{"starts with dash", "-mydb", true},
		{"starts with dot", ".mydb", true},
		{"contains space", "my db", true},
		{"contains special char", "my$db", true},
		{"potential flag", "--all-databases", true},
		{"too long", string(make([]byte, 65)), true},
		{"sql injection attempt", "mydb'; DROP TABLE users--", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDatabaseName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDatabaseName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateTableName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "users", false},
		{"valid with schema", "public.users", false},
		{"valid with underscore", "user_data", false},
		{"empty", "", true},
		{"starts with dash", "-table", true},
		{"starts with dot", ".table", true},
		{"contains space", "my table", true},
		{"sql injection", "users'; DROP--", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTableName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTableName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateBackupID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "backup-2025-12-29-10-30-45-abc12345", false},
		{"empty", "", true},
		{"path traversal dots", "backup-../../etc/passwd", true},
		{"path traversal slash", "backup-2025/../../etc", true},
		{"backslash", "backup-2025\\test", true},
		{"invalid format", "invalid-id", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBackupID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBackupID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		baseDir string
		wantErr bool
	}{
		{"clean path", "/tmp/backup.sql", "", false},
		{"path with dots", "/tmp/./backup.sql", "", false},
		{"within base dir", "/var/lib/db-backup/backups/test.sql", "/var/lib/db-backup", false},
		{"path traversal", "/var/lib/../../../etc/passwd", "/var/lib/db-backup", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SanitizePath(tt.path, tt.baseDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("SanitizePath(%q, %q) error = %v, wantErr %v", tt.path, tt.baseDir, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name    string
		port    int
		wantErr bool
	}{
		{"valid mysql", 3306, false},
		{"valid postgres", 5432, false},
		{"valid max", 65535, false},
		{"valid min", 1, false},
		{"zero", 0, true},
		{"negative", -1, true},
		{"too large", 65536, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePort(tt.port)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePort(%d) error = %v, wantErr %v", tt.port, err, tt.wantErr)
			}
		})
	}
}

func TestValidateCompressionLevel(t *testing.T) {
	tests := []struct {
		name    string
		level   int
		wantErr bool
	}{
		{"valid default", -1, false},
		{"valid 0", 0, false},
		{"valid 6", 6, false},
		{"valid 9", 9, false},
		{"too low", -2, true},
		{"too high", 10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCompressionLevel(tt.level)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCompressionLevel(%d) error = %v, wantErr %v", tt.level, err, tt.wantErr)
			}
		})
	}
}

func TestValidateEncryptionKeyLength(t *testing.T) {
	tests := []struct {
		name      string
		key       []byte
		minLength int
		wantErr   bool
	}{
		{"valid 32", make([]byte, 32), 32, false},
		{"too short", make([]byte, 16), 32, true},
		{"empty", []byte{}, 16, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEncryptionKeyLength(tt.key, tt.minLength)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEncryptionKeyLength() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
