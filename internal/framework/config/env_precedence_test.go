package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDotEnvCannotReplaceExplicitProcessConfiguration(t *testing.T) {
	if os.Getenv("CONFIG_PRECEDENCE_CHILD") == "1" {
		if os.Getenv("CONFIG_PRECEDENCE_VALUE") != "explicit-process" || os.Getenv("CONFIG_PRECEDENCE_DEFAULT") != "file-default" {
			os.Exit(2)
		}
		return
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, ".env"), []byte("CONFIG_PRECEDENCE_VALUE=file-override\nCONFIG_PRECEDENCE_DEFAULT=file-default\n"), 0600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(executable, "-test.run=^TestDotEnvCannotReplaceExplicitProcessConfiguration$")
	child.Dir = directory
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "CONFIG_PRECEDENCE_") {
			child.Env = append(child.Env, value)
		}
	}
	child.Env = append(child.Env, "CONFIG_PRECEDENCE_CHILD=1", "CONFIG_PRECEDENCE_VALUE=explicit-process")
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("dotenv initialization replaced explicit process settings: %v %s", err, output)
	}
}
