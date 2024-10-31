package testhelper

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
)

func BuildBinary(name string) (string, error) {
	_, filename, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(filename), name)
	aout := filepath.Join(dir, name)

	err := exec.Command("go", "build", "-o", aout, dir).Run()
	if err != nil {
		return "", fmt.Errorf("build command: %w", err)
	}

	return aout, nil
}
