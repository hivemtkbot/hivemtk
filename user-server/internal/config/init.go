package config

import (
	"os"
	"path/filepath"
)

func GetRootDir() string {
	executable, err := os.Executable()
	if err != nil {
		panic(err)
	}
	dir := filepath.Dir(executable)
	if _, err := os.Stat(filepath.Dir(dir)); os.IsNotExist(err) {
		err = os.MkdirAll(filepath.Dir(dir), 0755)
		if err != nil {
			panic(err)
		}
	}
	return dir
}

func GetEnvDir() string {
	envDir := filepath.Join(GetRootDir(), ".env")
	parent := filepath.Dir(envDir)
	if _, err := os.Stat(parent); os.IsNotExist(err) {
		if err := os.MkdirAll(parent, 0755); err != nil {
			panic(err)
		}
	}
	if _, err := os.Stat(envDir); os.IsNotExist(err) {
		if err := os.MkdirAll(envDir, 0755); err != nil {
			panic(err)
		}
	}
	return envDir
}

