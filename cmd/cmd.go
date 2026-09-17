package cmd

import "io"

// Execute runs the command tree with the process arguments and streams.
func Execute() error {
	return NewRootCmd().Execute()
}

// ExecuteWithArgs runs a fresh command tree with supplied arguments and output.
func ExecuteWithArgs(args []string, out io.Writer) error {
	root := NewRootCmd()
	root.SetArgs(args)
	root.SetOut(out)
	root.SetErr(out)
	return root.Execute()
}
