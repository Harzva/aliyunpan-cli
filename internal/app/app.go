package app

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

type App struct {
	in     io.Reader
	out    io.Writer
	errOut io.Writer
}

func New(in io.Reader, out io.Writer, errOut io.Writer) *App {
	return &App{in: in, out: out, errOut: errOut}
}

func (a *App) Run(args []string) error {
	opts := OutputOptions{Format: "json"}
	args, err := parseLeadingGlobal(args, &opts)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		a.printHelp()
		return usageError("missing command")
	}

	switch args[0] {
	case "help", "-h", "--help":
		a.printHelp()
		return nil
	case "auth":
		return a.runAuth(args[1:], opts)
	case "whoami":
		return a.runWhoami(args[1:], opts)
	case "drive":
		return a.runDrive(args[1:], opts)
	case "ls":
		return a.runLS(args[1:], opts)
	case "stat":
		return a.runStat(args[1:], opts)
	case "mkdir":
		return a.runMkdir(args[1:], opts)
	case "upload":
		return a.runUpload(args[1:], opts)
	case "download":
		return a.runDownload(args[1:], opts)
	case "rm":
		return a.runRM(args[1:], opts)
	default:
		return usageError("unknown command %q", args[0])
	}
}

func (a *App) PrintError(err error) {
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		cliErr = &CLIError{Code: "internal_error", Message: err.Error(), Status: exitInternal}
	}
	_ = writeOutput(a.out, "json", errorOutput{Error: *cliErr})
	if cliErr.Err != nil {
		fmt.Fprintf(a.errOut, "error[%s]: %s: %v\n", cliErr.Code, cliErr.Message, cliErr.Err)
	} else {
		fmt.Fprintf(a.errOut, "error[%s]: %s\n", cliErr.Code, cliErr.Message)
	}
}

func (a *App) printHelp() {
	fmt.Fprintln(a.errOut, `aliyunpan-cli

Usage:
  aliyunpan-cli auth login [flags]
  aliyunpan-cli auth import [flags]
  aliyunpan-cli whoami [flags]
  aliyunpan-cli drive list [flags]
  aliyunpan-cli drive use <file|resource|drive-id> [flags]
  aliyunpan-cli ls [path] [flags]
  aliyunpan-cli stat <path> [flags]
  aliyunpan-cli mkdir <path> [flags]
  aliyunpan-cli upload <local-file> <remote-dir> [flags]
  aliyunpan-cli download <remote-file> [flags]
  aliyunpan-cli rm <path...> [flags]

Common flags:
  --format json|table|csv
  --json
  --no-progress
  --config-dir PATH`)
}

func parseLeadingGlobal(args []string, opts *OutputOptions) ([]string, error) {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			opts.JSONFlag = true
			opts.Format = "json"
		case arg == "--no-progress":
			opts.NoProgress = true
		case arg == "--format":
			i++
			if i >= len(args) {
				return nil, usageError("--format requires a value")
			}
			opts.Format = args[i]
			opts.ExplicitTable = opts.Format == "table"
		case strings.HasPrefix(arg, "--format="):
			opts.Format = strings.TrimPrefix(arg, "--format=")
			opts.ExplicitTable = opts.Format == "table"
		case arg == "--config-dir":
			i++
			if i >= len(args) {
				return nil, usageError("--config-dir requires a value")
			}
			opts.ConfigDir = args[i]
		case strings.HasPrefix(arg, "--config-dir="):
			opts.ConfigDir = strings.TrimPrefix(arg, "--config-dir=")
		default:
			out = append(out, args[i:]...)
			return out, nil
		}
	}
	return out, nil
}

func newFlagSet(name string, errOut io.Writer, opts *OutputOptions) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.StringVar(&opts.Format, "format", opts.Format, "output format: json, table, csv")
	fs.BoolVar(&opts.JSONFlag, "json", opts.JSONFlag, "force JSON output and suppress progress")
	fs.BoolVar(&opts.NoProgress, "no-progress", opts.NoProgress, "suppress transfer progress")
	fs.StringVar(&opts.ConfigDir, "config-dir", opts.ConfigDir, "config directory")
	return fs
}

func parseFlagSet(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return usageError(err.Error())
	}
	return nil
}

func shouldShowProgress(opts OutputOptions) bool {
	return !opts.NoProgress && !opts.JSONFlag
}
