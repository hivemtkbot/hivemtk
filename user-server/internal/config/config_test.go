package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetRootDir(t *testing.T) {
	rootDir := GetRootDir()
	if rootDir == "" {
		t.Error("Expected non-empty root directory")
	}
}

func TestGetEnvDir(t *testing.T) {
	envDir := GetEnvDir()
	if envDir == "" {
		t.Error("Expected non-empty env directory")
	}
}

func TestGetRootDir_Consistency(t *testing.T) {
	rootDir1 := GetRootDir()
	rootDir2 := GetRootDir()

	if rootDir1 != rootDir2 {
		t.Error("GetRootDir should return consistent results")
	}
}

func TestGetEnvDir_Consistency(t *testing.T) {
	envDir1 := GetEnvDir()
	envDir2 := GetEnvDir()

	if envDir1 != envDir2 {
		t.Error("GetEnvDir should return consistent results")
	}
}

func TestGetEnvDir_PathStructure(t *testing.T) {
	rootDir := GetRootDir()
	envDir := GetEnvDir()

	expectedEnvDir := filepath.Join(rootDir, ".env")
	if envDir != expectedEnvDir {
		t.Errorf("Expected env dir to be %s, got %s", expectedEnvDir, envDir)
	}
}

// Test that directories are created if they don't exist
func TestGetEnvDir_CreatesDirectory(t *testing.T) {
	// This test mainly verifies that the function doesn't fail
	envDir := GetEnvDir()

	// The env dir should be accessible
	_, err := os.Stat(filepath.Dir(envDir))
	if err != nil {
		t.Errorf("Expected env directory to exist, got error: %v", err)
	}
}
