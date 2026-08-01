package cli

import (
	"context"
	"os"
	"testing"
)

const stopCleanupHelperProcessEnv = "FLEDGE_TEST_STOP_CLEANUP_HELPER_PROCESS"

func TestMain(m *testing.M) {
	if os.Getenv(stopCleanupHelperProcessEnv) == "1" &&
		len(os.Args) > 1 && os.Args[1] == stopCleanupCommand {
		os.Exit(Execute(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

func enableStopCleanupHelperProcess(t *testing.T) {
	t.Helper()
	t.Setenv(stopCleanupHelperProcessEnv, "1")
}
