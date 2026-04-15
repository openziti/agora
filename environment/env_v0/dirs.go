package env_v0

import (
	"fmt"
	"os"
	"path/filepath"
)

var rootDirName = ".agora"

func SetRootDirName(name string) {
	rootDirName = name
}

func rootDir() (string, error) {
	if filepath.IsAbs(rootDirName) {
		return rootDirName, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, rootDirName), nil
}

func metadataFile() (string, error) {
	root, err := rootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "metadata.json"), nil
}

func configFile() (string, error) {
	root, err := rootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "config.json"), nil
}

func environmentFile() (string, error) {
	root, err := rootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "environment.json"), nil
}

func networkFile() (string, error) {
	root, err := rootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "network.json"), nil
}

func networkSocketFile() (string, error) {
	root, err := rootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "network.sock"), nil
}

func identitiesDir() (string, error) {
	root, err := rootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "identities"), nil
}

func identityFile(name string) (string, error) {
	dir, err := identitiesDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("%s.json", name)), nil
}
