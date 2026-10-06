package agent

import "github.com/Harrison-Blair/fledge/internal/lib/cli"

// Temporary aliases while callers move to lib/cli.
type (
	Outcome       = cli.Outcome
	Failure       = cli.Failure
	Effect        = cli.Effect
	HumanRenderer = cli.HumanRenderer
	InputError    = cli.InputError
	ResultError   = cli.ResultError
	TextInput     = cli.TextInput
)

var (
	NewOutcome     = cli.NewOutcome
	Invalid        = cli.Invalid
	InvalidOutcome = cli.InvalidOutcome
	Finish         = cli.Finish
	ReadText       = cli.ReadText
	AtPhase        = cli.AtPhase
	Pointer        = cli.Pointer
	Display        = cli.Display
	DisplayString  = cli.DisplayString
)
