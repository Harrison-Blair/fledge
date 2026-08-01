package cli

import (
	"errors"
	"fmt"

	"github.com/Harrison-Blair/fledge/internal/agentprofile"
	"github.com/Harrison-Blair/fledge/internal/agentspawn"
)

const orchestratorProfileName = "orchestrator"

// startupOrchestratorProfile returns the managed profile to inject at fresh
// attached startup. A missing profile intentionally selects the legacy ad-hoc
// picker without a warning; every unusable profile returns a warning and the
// same fallback.
func startupOrchestratorProfile(env *environment, projectRoot string) (string, error) {
	store, err := agentprofile.New(projectRoot)
	if err != nil {
		return "", fmt.Errorf("open orchestrator profile store: %w", err)
	}
	profile, loadErr := store.Load(orchestratorProfileName)
	closeErr := store.Close()
	if errors.Is(loadErr, agentprofile.ErrNotFound) {
		if closeErr != nil {
			return "", fmt.Errorf("close orchestrator profile store: %w", closeErr)
		}
		return "", nil
	}
	if loadErr != nil {
		return "", fmt.Errorf("load orchestrator profile: %w", errors.Join(loadErr, closeErr))
	}
	if closeErr != nil {
		return "", fmt.Errorf("close orchestrator profile store: %w", closeErr)
	}
	installed := agentspawn.Installed(env.lookPath)
	if len(compatibleProfileHarnesses(installed, profile)) == 0 {
		return "", errors.New("orchestrator profile has no compatible installed harness")
	}
	return orchestratorProfileName, nil
}
