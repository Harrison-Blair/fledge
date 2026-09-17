# Fledge

A Go CLI scaffold built with Cobra.

```sh
go run . --help
go run . --version
go install .
```

## Layout

- `main.go` delegates to `cmd.Execute()` and handles the exit status.
- `cmd/` constructs fresh command trees with `NewRootCmd()` and provides
  `ExecuteWithArgs()` for tests.
- `cmd/<name>/` owns Cobra wiring; `internal/<name>/` owns the implementation.
- `internal/version/VERSION` is the sole release version source, embedded in the
  binary and exposed through `--version` and `-V`.

New subcommands export `New() *cobra.Command` and are registered by their parent.
Keep application logic in `internal/`, independent of Cobra.

## Development

```sh
gofmt -l .
go vet ./...
go test -race ./...
go build -o /tmp/fledge .
```

The existing GitHub workflows lint, test, and build for Linux amd64 and arm64.
Merges to `main` may create or refresh a release draft using the version file;
they do not publish releases automatically.

## License

[MIT](LICENSE).
