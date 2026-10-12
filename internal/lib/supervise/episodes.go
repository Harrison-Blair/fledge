package supervise

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Harrison-Blair/fledge/internal/lib/state"
)

const episodesName = "episodes.json"

// Episodes records the wakes already sent, so a restarted daemon does not
// repeat them. Blocked and Quiet hold the agent record ids whose current
// blocked or idle episode has had its wake; they are cleared when the episode
// ends. Exited holds agents whose exit was reported. Rerung maps an inbox id
// to when its doorbell was rung again; Unacknowledged holds inbox ids whose
// sender was told they went unread.
type Episodes struct {
	Blocked        map[string]bool   `json:"blocked"`
	Quiet          map[string]bool   `json:"quiet"`
	Exited         map[string]bool   `json:"exited"`
	Rerung         map[string]string `json:"rerung"`
	Unacknowledged map[string]bool   `json:"unacknowledged"`
}

// LoadEpisodes reads the episodes in dir; a missing file is none.
func LoadEpisodes(dir string) (Episodes, error) {
	e := Episodes{}
	data, err := os.ReadFile(filepath.Join(dir, episodesName))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return e.init(), fmt.Errorf("supervise: read episodes: %w", err)
	}
	if err == nil {
		if err := json.Unmarshal(data, &e); err != nil {
			return Episodes{}.init(), fmt.Errorf("supervise: decode episodes: %w", err)
		}
	}
	return e.init(), nil
}

func (e Episodes) init() Episodes {
	if e.Blocked == nil {
		e.Blocked = map[string]bool{}
	}
	if e.Quiet == nil {
		e.Quiet = map[string]bool{}
	}
	if e.Exited == nil {
		e.Exited = map[string]bool{}
	}
	if e.Rerung == nil {
		e.Rerung = map[string]string{}
	}
	if e.Unacknowledged == nil {
		e.Unacknowledged = map[string]bool{}
	}
	return e
}

// Save atomically replaces the episodes in dir.
func (e Episodes) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("supervise: create %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	return state.WriteReplace(filepath.Join(dir, episodesName), append(data, '\n'))
}
