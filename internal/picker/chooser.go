package picker

import (
	"context"
	"fmt"
	"io"

	"fledge/internal/catalog"
	"fledge/internal/profile"
	"fledge/internal/session"
)

const (
	harnessPrompt  = "Select harness"
	shellOnlyTitle = "none — shell only"
	defaultTitle   = "harness default"
	freeTextTitle  = "enter model ID"
	profilePrompt  = "Select agent profile"
	noProfileTitle = "None"
)

// LaunchRequest contains the unresolved choices supplied by a command. Empty
// Harness, Model, and Profile values are resolved from profile defaults and,
// when Interactive is true, terminal prompts.
type LaunchRequest struct {
	Harness string
	Model   string
	Profile string

	NoProfile      bool
	DefaultProfile string
	Args           []string

	PromptProfile  bool
	AllowShellOnly bool
	Interactive    bool
}

// LaunchChoice is a fully resolved agent launch. An empty Harness denotes the
// shell-only choice. Profile is the immutable managed-profile snapshot to pin
// to the launched agent; nil means no profile.
type LaunchChoice struct {
	Harness catalog.Harness
	Model   string
	Profile *profile.Profile
	Args    []string
}

// SelectFunc presents one terminal selection.
type SelectFunc func(io.Reader, io.Writer, string, []Option) (Option, error)

// Resolver applies launch precedence and prompts for choices that remain
// unresolved. Prompt functions are injectable so callers and tests do not
// depend on a particular terminal implementation.
type Resolver struct {
	Input  io.Reader
	Output io.Writer

	// Models reports the model IDs a harness accepts.
	Models func(context.Context, catalog.Harness) []string
	Select SelectFunc

	// Profile registry seams used by package tests.
	profilesFn func() []profile.Profile
	profileFn  func(string) (profile.Profile, bool)
}

// Resolve resolves a launch request. Explicit harness and model values win
// over profile defaults. Any value still missing is prompted for only in
// interactive mode; non-interactive requests must be complete.
func (r Resolver) Resolve(ctx context.Context, request LaunchRequest) (LaunchChoice, error) {
	configured, err := r.resolveProfile(request)
	if err != nil {
		return LaunchChoice{}, err
	}

	harnessName := request.Harness
	if harnessName == "" && configured != nil {
		harnessName = configured.Defaults.Harness
	}

	var harness catalog.Harness
	if harnessName != "" {
		harness, err = catalog.ParseHarness(harnessName)
		if err != nil {
			return LaunchChoice{}, fmt.Errorf("resolve launch: %w", err)
		}
	} else {
		if !request.Interactive {
			return LaunchChoice{}, fmt.Errorf("resolve launch: harness is required in non-interactive mode; pass --harness")
		}
		harness, err = r.promptHarness(request.AllowShellOnly)
		if err != nil {
			return LaunchChoice{}, fmt.Errorf("resolve launch: choose harness: %w", err)
		}
		if harness == "" {
			return LaunchChoice{}, nil
		}
	}

	model := request.Model
	if model == "" && configured != nil {
		model = configured.Defaults.Model
	}
	if model == "" {
		if !request.Interactive {
			return LaunchChoice{}, fmt.Errorf("resolve launch: model is required in non-interactive mode; pass --model")
		}
		model, err = r.promptModel(ctx, harness)
		if err != nil {
			return LaunchChoice{}, fmt.Errorf("resolve launch: choose model: %w", err)
		}
	}

	args := append([]string(nil), request.Args...)
	if configured != nil {
		args = append(append([]string(nil), configured.Defaults.Args...), args...)
		// Delivery itself is applied after the caller has persisted the profile
		// snapshot. A placeholder path is enough to validate reserved native
		// instruction arguments and harness support here.
		if _, err := profile.LaunchArgs(*configured, string(harness), "/fledge/profile/instructions.md", args); err != nil {
			return LaunchChoice{}, fmt.Errorf("resolve launch: %w", err)
		}
	}

	return LaunchChoice{
		Harness: harness,
		Model:   model,
		Profile: configured,
		Args:    args,
	}, nil
}

func (r Resolver) resolveProfile(request LaunchRequest) (*profile.Profile, error) {
	if request.NoProfile && request.Profile != "" {
		return nil, fmt.Errorf("resolve launch: --profile and --no-profile cannot be used together")
	}
	if request.Profile == "none" {
		return nil, fmt.Errorf("resolve launch: profile name %q is reserved; use --no-profile", "none")
	}
	if request.NoProfile {
		return nil, nil
	}
	if request.DefaultProfile == "none" {
		return nil, fmt.Errorf("resolve launch: profile name %q is reserved; use --no-profile", "none")
	}

	name := request.Profile
	if name == "" {
		name = request.DefaultProfile
	}
	if name == "" && request.PromptProfile && request.Interactive {
		chosen, err := r.selectOne(profilePrompt, r.profileOptions())
		if err != nil {
			return nil, fmt.Errorf("resolve launch: choose profile: %w", err)
		}
		name = chosen.ID
	}
	if name == "" {
		return nil, nil
	}

	configured, ok := r.getProfile(name)
	if !ok {
		return nil, fmt.Errorf("resolve launch: unknown profile %q", name)
	}
	return &configured, nil
}

func (r Resolver) getProfile(name string) (profile.Profile, bool) {
	get := r.profileFn
	if get == nil {
		get = profile.Get
	}
	return get(name)
}

func (r Resolver) profileOptions() []Option {
	list := r.profilesFn
	if list == nil {
		list = profile.List
	}
	managed := list()
	options := make([]Option, 0, len(managed)+1)
	options = append(options, Option{Title: noProfileTitle})
	for _, configured := range managed {
		options = append(options, Option{
			ID:          configured.Name,
			Title:       configured.Name,
			Description: configured.Description,
		})
	}
	return options
}

func (r Resolver) promptHarness(allowShellOnly bool) (catalog.Harness, error) {
	harnesses := catalog.Harnesses()
	options := make([]Option, 0, len(harnesses)+1)
	for _, harness := range harnesses {
		options = append(options, Option{ID: string(harness), Title: string(harness)})
	}
	if allowShellOnly {
		options = append(options, Option{Title: shellOnlyTitle})
	}

	chosen, err := r.selectOne(harnessPrompt, options)
	if err != nil {
		return "", err
	}
	if chosen.ID == "" {
		return "", nil
	}
	harness, err := catalog.ParseHarness(chosen.ID)
	if err != nil {
		return "", err
	}
	return harness, nil
}

func (r Resolver) promptModel(ctx context.Context, harness catalog.Harness) (string, error) {
	if r.Models == nil {
		return "", fmt.Errorf("model lookup is nil")
	}
	models := r.Models(ctx, harness)
	if err := ctx.Err(); err != nil {
		return "", err
	}

	options := make([]Option, 0, len(models)+2)
	options = append(options,
		Option{Title: defaultTitle},
		Option{Title: freeTextTitle, FreeText: true},
	)
	for _, id := range models {
		options = append(options, Option{ID: id, Title: id})
	}
	chosen, err := r.selectOne("Model for "+string(harness), options)
	if err != nil {
		return "", err
	}
	return chosen.ID, nil
}

func (r Resolver) selectOne(title string, options []Option) (Option, error) {
	if r.Input == nil {
		return Option{}, fmt.Errorf("terminal input is nil")
	}
	if r.Output == nil {
		return Option{}, fmt.Errorf("terminal output is nil")
	}
	choose := r.Select
	if choose == nil {
		choose = Select
	}
	return choose(r.Input, r.Output, title, options)
}

// SessionChooser adapts one launch request to the session lifecycle's Chooser
// contract. Start command adapters should use this type so the resolved
// profile snapshot and native arguments are persisted with the session.
type SessionChooser struct {
	Resolver Resolver
	Request  LaunchRequest
}

// Choose resolves and converts the configured request for session startup.
func (c SessionChooser) Choose(ctx context.Context) (session.AgentChoice, error) {
	choice, err := c.Resolver.Resolve(ctx, c.Request)
	if err != nil {
		return session.AgentChoice{}, err
	}
	return sessionChoice(choice), nil
}

func sessionChoice(choice LaunchChoice) session.AgentChoice {
	return session.AgentChoice{
		Harness: string(choice.Harness),
		Model:   choice.Model,
		Args:    append([]string(nil), choice.Args...),
		Profile: choice.Profile,
	}
}
