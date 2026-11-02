package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jdtobe/gavel/internal/app"
	"github.com/jdtobe/gavel/internal/config"
	appruntime "github.com/jdtobe/gavel/internal/runtime"
	"github.com/jdtobe/gavel/internal/sidecar"
	"github.com/jdtobe/gavel/internal/tui"
	"github.com/jdtobe/gavel/internal/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "gavel:", err)
		os.Exit(1)
	}
}

type runOptions struct {
	out          io.Writer
	goos, goarch string
	launch       func(context.Context, *app.App, *appruntime.Setup, bool) error
}

func run(args []string) error {
	return runWith(args, runOptions{out: os.Stdout, goos: runtime.GOOS, goarch: runtime.GOARCH, launch: launchTUI})
}
func runWith(args []string, options runOptions) error {
	flags := flag.NewFlagSet("gavel", flag.ContinueOnError)
	flags.SetOutput(options.out)
	showVersion := flags.Bool("version", false, "print the build version")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if *showVersion {
		fmt.Fprintf(options.out, "gavel %s\n", version.Version)
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if options.goos != "darwin" || options.goarch != "arm64" {
		return fmt.Errorf("Gavel requires macOS on Apple Silicon (arm64)")
	}
	paths, err := appruntime.ResolvePaths()
	if err != nil {
		return err
	}
	for _, path := range []string{paths.RuntimeDir, paths.LogsDir, paths.CacheDir} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
	}
	release, err := sidecar.AcquireLock(paths.LockFile)
	if err != nil {
		return err
	}
	defer release()
	if err := sidecar.ReapOrphan(paths.SidecarStateFile); err != nil {
		return err
	}
	cfg, err := config.Load(paths.ConfigFile)
	if err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(paths.LogsDir, "kev.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer log.Close()
	setup := appruntime.NewSetup(paths)
	setup.Log = log
	manager := &sidecar.Manager{Paths: paths, Python: setup.VenvPython(), Log: log}
	a := app.New(app.Deps{Paths: paths, Config: cfg, Runtime: setup, Sidecar: manager})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() {
		stopCtx, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		a.Stop(stopCtx)
	}()
	if _, err := a.StartConsole(ctx); err != nil {
		return err
	}
	auto := false
	for _, model := range cfg.DownloadedModels {
		if model == cfg.Model {
			auto = setup.Installed()
		}
	}
	return options.launch(ctx, a, setup, auto)
}
func launchTUI(ctx context.Context, a *app.App, s *appruntime.Setup, auto bool) error {
	m := tui.New(tui.Deps{App: a, MemoryBytes: s.MemoryBytes, DiskFree: s.DiskFree, DiskPath: s.Paths.RuntimeDir, AutoStart: auto})
	_, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	return err
}
