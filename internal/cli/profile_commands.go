package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"unicode"

	"github.com/Harrison-Blair/fledge/internal/agentprofile"
	"github.com/Harrison-Blair/fledge/internal/fledge"
	"github.com/Harrison-Blair/fledge/internal/ui"
	"github.com/pelletier/go-toml"
	"github.com/spf13/cobra"
)

const maxProfileInputSize = 1 << 20

type profileFields struct {
	description     string
	harness         string
	model           string
	effort          string
	instructions    string
	nativeArgs      []string
	file            string
	descriptionSet  bool
	harnessSet      bool
	modelSet        bool
	effortSet       bool
	instructionsSet bool
	nativeArgsSet   bool
}

type profileMutationResult struct {
	Profile agentprofile.Profile `json:"profile"`
}

type profileListResult struct {
	Profiles []agentprofile.Profile `json:"profiles"`
}

type profileValidationResult struct {
	Valid   bool                 `json:"valid"`
	Profile agentprofile.Profile `json:"profile"`
}

type profileDeleteResult struct {
	Name    string `json:"name"`
	Deleted bool   `json:"deleted"`
}

func newAgentProfile(env *environment) *cobra.Command {
	cmd := &cobra.Command{Use: "profile", Short: "Manage deterministic agent profiles"}
	cmd.AddCommand(
		newAgentProfileCreate(env),
		newAgentProfileUpdate(env),
		newAgentProfileList(env),
		newAgentProfileShow(env),
		newAgentProfileValidate(env),
		newAgentProfileDelete(env),
	)
	return cmd
}

func newAgentProfileCreate(env *environment) *cobra.Command {
	var fields profileFields
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an agent profile",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fields.captureChanged(cmd)
			profile, err := profileFromInput(env, args[0], fields.file)
			if err != nil {
				return err
			}
			fields.apply(&profile)
			if err := agentprofile.Validate(profile); err != nil {
				return translateProfileError(err)
			}
			return withProfileStore(env, func(store *agentprofile.Store) error {
				created, err := store.Create(profile)
				if err != nil {
					return translateProfileError(err)
				}
				return printProfileMutation(env, "Created", created)
			})
		},
	}
	bindProfileFields(cmd, &fields)
	return cmd
}

func newAgentProfileUpdate(env *environment) *cobra.Command {
	var fields profileFields
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update an agent profile atomically",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fields.captureChanged(cmd)
			return withProfileStore(env, func(store *agentprofile.Store) error {
				var profile agentprofile.Profile
				var err error
				if fields.file == "" {
					profile, err = store.Load(args[0])
				} else {
					profile, err = profileFromInput(env, args[0], fields.file)
				}
				if err != nil {
					return translateProfileError(err)
				}
				fields.apply(&profile)
				if err := agentprofile.Validate(profile); err != nil {
					return translateProfileError(err)
				}
				updated, err := store.Update(profile)
				if err != nil {
					return translateProfileError(err)
				}
				return printProfileMutation(env, "Updated", updated)
			})
		},
	}
	bindProfileFields(cmd, &fields)
	return cmd
}

func newAgentProfileList(env *environment) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List agent profiles",
		Args:  noArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return withProfileStore(env, func(store *agentprofile.Store) error {
				profiles, err := store.List()
				if err != nil {
					return translateProfileError(err)
				}
				return env.print(profileListResult{Profiles: profiles}, func(w io.Writer, theme *ui.Theme) {
					printProfiles(w, profiles, theme)
				})
			})
		},
	}
}

func newAgentProfileShow(env *environment) *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show an agent profile",
		Args:  exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withProfileStore(env, func(store *agentprofile.Store) error {
				profile, err := store.Show(args[0])
				if err != nil {
					return translateProfileError(err)
				}
				return env.print(profileMutationResult{Profile: profile}, func(w io.Writer, theme *ui.Theme) {
					printProfile(w, profile, theme)
				})
			})
		},
	}
}

func newAgentProfileValidate(env *environment) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "validate <name>",
		Short: "Strictly validate a stored profile or TOML input",
		Args:  exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withProfileStore(env, func(store *agentprofile.Store) error {
				var profile agentprofile.Profile
				var err error
				if file == "" {
					profile, err = store.Load(args[0])
				} else {
					profile, err = profileFromInput(env, args[0], file)
					if err == nil {
						err = agentprofile.Validate(profile)
					}
				}
				if err != nil {
					return translateProfileError(err)
				}
				result := profileValidationResult{Valid: true, Profile: profile}
				return env.print(result, func(w io.Writer, theme *ui.Theme) {
					fmt.Fprintf(w, "%s agent profile %s\n", theme.Accent("Valid"), terminalSafeText(profile.Name))
				})
			})
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "validate TOML from a file, or - for stdin")
	return cmd
}

func newAgentProfileDelete(env *environment) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an agent profile",
		Args:  exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withProfileStore(env, func(store *agentprofile.Store) error {
				if err := store.Delete(args[0]); err != nil {
					return translateProfileError(err)
				}
				result := profileDeleteResult{Name: args[0], Deleted: true}
				return env.print(result, func(w io.Writer, theme *ui.Theme) {
					fmt.Fprintf(w, "%s agent profile %s\n", theme.Accent("Deleted"), terminalSafeText(args[0]))
				})
			})
		},
	}
}

func bindProfileFields(cmd *cobra.Command, fields *profileFields) {
	cmd.Flags().StringVarP(&fields.file, "file", "f", "", "read base TOML from a file, or - for stdin")
	cmd.Flags().StringVarP(&fields.description, "description", "d", "", "profile description")
	cmd.Flags().StringVarP(&fields.harness, "harness", "k", "", "agent harness")
	cmd.Flags().StringVarP(&fields.model, "model", "m", "", "model launch ID")
	cmd.Flags().StringVarP(&fields.effort, "effort", "e", "", "reasoning effort")
	cmd.Flags().StringVarP(&fields.instructions, "instructions", "i", "", "managed agent instructions")
	cmd.Flags().StringArrayVarP(&fields.nativeArgs, "native-arg", "a", nil, "native harness argument (repeatable)")
}

func (fields *profileFields) captureChanged(cmd *cobra.Command) {
	fields.descriptionSet = cmd.Flags().Changed("description")
	fields.harnessSet = cmd.Flags().Changed("harness")
	fields.modelSet = cmd.Flags().Changed("model")
	fields.effortSet = cmd.Flags().Changed("effort")
	fields.instructionsSet = cmd.Flags().Changed("instructions")
	fields.nativeArgsSet = cmd.Flags().Changed("native-arg")
}

func (fields profileFields) apply(profile *agentprofile.Profile) {
	if fields.descriptionSet {
		profile.Description = fields.description
	}
	if fields.harnessSet {
		profile.Harness = fields.harness
	}
	if fields.modelSet {
		profile.Model = fields.model
	}
	if fields.effortSet {
		profile.Effort = fields.effort
	}
	if fields.instructionsSet {
		profile.Instructions = fields.instructions
	}
	if fields.nativeArgsSet {
		profile.NativeArgs = append([]string(nil), fields.nativeArgs...)
	}
}

func profileFromInput(env *environment, name, path string) (agentprofile.Profile, error) {
	profile := agentprofile.Profile{
		Name: name, SchemaVersion: agentprofile.SchemaVersion, NativeArgs: []string{},
	}
	if path == "" {
		return profile, nil
	}
	data, err := readProfileInput(env, path)
	if err != nil {
		return agentprofile.Profile{}, err
	}
	if err := toml.NewDecoder(bytes.NewReader(data)).Strict(true).Decode(&profile); err != nil {
		return agentprofile.Profile{}, fledge.Wrap("profile_invalid", fmt.Sprintf("decode profile TOML: %v", err), err)
	}
	profile.Name = name
	if profile.NativeArgs == nil {
		profile.NativeArgs = []string{}
	}
	return profile, nil
}

func readProfileInput(env *environment, path string) ([]byte, error) {
	var reader io.Reader
	var closeFile *os.File
	if path == "-" {
		reader = env.in
	} else {
		file, err := os.Open(path)
		if err != nil {
			return nil, fledge.Wrap("profile_input_failed", fmt.Sprintf("open profile input %q: %v", path, err), err)
		}
		reader, closeFile = file, file
	}
	if reader == nil {
		reader = strings.NewReader("")
	}
	data, readErr := io.ReadAll(io.LimitReader(reader, maxProfileInputSize+1))
	var closeErr error
	if closeFile != nil {
		closeErr = closeFile.Close()
	}
	if readErr != nil || closeErr != nil {
		err := errors.Join(readErr, closeErr)
		return nil, fledge.Wrap("profile_input_failed", fmt.Sprintf("read profile input %q: %v", path, err), err)
	}
	if len(data) > maxProfileInputSize {
		return nil, fledge.NewError("profile_invalid", fmt.Sprintf("profile input exceeds %d bytes", maxProfileInputSize))
	}
	return data, nil
}

func withProfileStore(env *environment, fn func(*agentprofile.Store) error) (resultErr error) {
	cwd, err := env.workingDirectory()
	if err != nil {
		return err
	}
	projectInfo, err := discoverProject(cwd)
	if err != nil {
		return err
	}
	store, err := agentprofile.New(projectInfo.Root)
	if err != nil {
		return translateProfileError(err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil && resultErr == nil {
			resultErr = fledge.Wrap("profile_store_failed", fmt.Sprintf("close agent profile store: %v", closeErr), closeErr)
		}
	}()
	return fn(store)
}

func translateProfileError(err error) error {
	if err == nil {
		return nil
	}
	var existing *fledge.Error
	if errors.As(err, &existing) {
		return existing
	}
	details := map[string]string{}
	var profileErr *agentprofile.Error
	if errors.As(err, &profileErr) {
		if profileErr.Op != "" {
			details["operation"] = profileErr.Op
		}
		if profileErr.Name != "" {
			details["name"] = profileErr.Name
		}
		if profileErr.Path != "" {
			details["path"] = profileErr.Path
		}
	}
	var validationErr *agentprofile.ValidationError
	if errors.As(err, &validationErr) && validationErr.Field != "" {
		details["field"] = validationErr.Field
	}
	code := "profile_store_failed"
	switch {
	case errors.Is(err, agentprofile.ErrAlreadyExists):
		code = "profile_already_exists"
	case errors.Is(err, agentprofile.ErrNotFound):
		code = "profile_not_found"
	case errors.Is(err, agentprofile.ErrInvalid):
		code = "profile_invalid"
	}
	translated := fledge.Wrap(code, err.Error(), err)
	if len(details) > 0 {
		translated.Details = details
	}
	return translated
}

func printProfileMutation(env *environment, action string, profile agentprofile.Profile) error {
	return env.print(profileMutationResult{Profile: profile}, func(w io.Writer, theme *ui.Theme) {
		fmt.Fprintf(w, "%s agent profile %s\n", theme.Accent(action), terminalSafeText(profile.Name))
	})
}

func printProfiles(w io.Writer, profiles []agentprofile.Profile, theme *ui.Theme) {
	if len(profiles) == 0 {
		fmt.Fprintln(w, "No agent profiles")
		return
	}
	table := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(table, theme.Accent("NAME\tHARNESS\tMODEL\tEFFORT\tDESCRIPTION"))
	for _, profile := range profiles {
		model, effort := profile.Model, profile.Effort
		if model == "" {
			model = "default"
		}
		if effort == "" {
			effort = "default"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n",
			terminalSafeText(profile.Name), terminalSafeText(profile.Harness),
			terminalSafeText(model), terminalSafeText(effort), terminalSafeText(profile.Description))
	}
	_ = table.Flush()
}

func printProfile(w io.Writer, profile agentprofile.Profile, theme *ui.Theme) {
	model, effort := profile.Model, profile.Effort
	if model == "" {
		model = "default"
	}
	if effort == "" {
		effort = "default"
	}
	fmt.Fprintf(w, "%s %s\n%s %d\n%s %s\n%s %s\n%s %s\n",
		theme.Accent("Profile:"), terminalSafeText(profile.Name),
		theme.Accent("Schema:"), profile.SchemaVersion,
		theme.Accent("Harness:"), terminalSafeText(profile.Harness),
		theme.Accent("Model:"), terminalSafeText(model),
		theme.Accent("Effort:"), terminalSafeText(effort))
	if profile.Description != "" {
		fmt.Fprintf(w, "%s %s\n", theme.Accent("Description:"), terminalSafeText(profile.Description))
	}
	if len(profile.NativeArgs) > 0 {
		fmt.Fprintf(w, "%s %s\n", theme.Accent("Native args:"), terminalSafeText(strings.Join(profile.NativeArgs, " ")))
	}
	if profile.Instructions != "" {
		fmt.Fprintf(w, "%s\n%s\n", theme.Accent("Instructions:"), terminalSafeText(profile.Instructions))
	}
}

// terminalSafeText preserves printable Unicode while making terminal control
// characters visible and inert. In particular, callers can safely place the
// result inside a tabwriter cell without allowing data to create columns or
// rows of its own.
func terminalSafeText(value string) string {
	var escaped strings.Builder
	for _, r := range value {
		switch r {
		case '\t':
			escaped.WriteString(`\t`)
		case '\n':
			escaped.WriteString(`\n`)
		case '\r':
			escaped.WriteString(`\r`)
		default:
			if unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp) {
				if r <= 0xff {
					fmt.Fprintf(&escaped, `\x%02x`, r)
				} else {
					fmt.Fprintf(&escaped, `\u%04x`, r)
				}
				continue
			}
			escaped.WriteRune(r)
		}
	}
	return escaped.String()
}
