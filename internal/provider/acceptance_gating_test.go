package provider_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestAcceptanceTestsSkipWithoutTFAcc(t *testing.T) {
	cmd := exec.Command("go", "test", ".", "-run", "^TestAccLocationResource_basic$", "-v")
	env := make([]string, 0, len(os.Environ()))
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "TF_ACC=") || strings.HasPrefix(value, "MISTERSHELL_URL=") || strings.HasPrefix(value, "MISTERSHELL_API_KEY=") {
			continue
		}
		env = append(env, value)
	}
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("acceptance subprocess failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "SKIP") {
		t.Fatalf("acceptance test did not skip without TF_ACC=1:\n%s", output)
	}
}
