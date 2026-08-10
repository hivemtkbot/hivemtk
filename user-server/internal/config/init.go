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
	// 检查 envDir 的父目录(executable 所在目录)是否存在
	parent := filepath.Dir(envDir)
	if _, err := os.Stat(parent); os.IsNotExist(err) {
		if err := os.MkdirAll(parent, 0755); err != nil {
			panic(err)
		}
	}
	// 确保 envDir(.env 目录)也存在
	if _, err := os.Stat(envDir); os.IsNotExist(err) {
		if err := os.MkdirAll(envDir, 0755); err != nil {
			panic(err)
		}
	}
	return envDir
}
