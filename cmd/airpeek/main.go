// Command airpeek is a read-only terminal browser for Airflow: DAGs, their
// runs, task instances and logs. It cannot trigger, pause, clear or
// otherwise change anything.
//
//	airpeek init     add an environment to the config file
//	airpeek          browse
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JakubPluta/airpeek/internal/config"
	"github.com/JakubPluta/airpeek/internal/tui"
)

// version is set at build time with -ldflags "-X main.version=v1.2.3";
// `go install …@vX` builds report the module version instead.
var version = "dev"

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	initCmd := len(args) > 0 && args[0] == "init"
	if initCmd {
		args = args[1:]
	}
	flags := flag.NewFlagSet("airpeek", flag.ExitOnError)
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage:\n  airpeek init [--config path]   add an Airflow environment\n  airpeek [flags]                browse\n\nFlags:")
		flags.PrintDefaults()
	}
	configPath := flags.String("config", "", "config file (default $AIRPEEK_CONFIG, else ~/.config/airpeek/config.yaml)")
	env := flags.String("env", "", "environment to open; picked from a list when empty")
	showVersion := flags.Bool("version", false, "print the version and exit")
	_ = flags.Parse(args)

	if *showVersion {
		fmt.Println(buildVersion())
		return 0
	}
	path := *configPath
	if path == "" {
		p, err := config.DefaultPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "airpeek: %v\n", err)
			return 1
		}
		path = p
	}

	if initCmd {
		if err := runInit(os.Stdin, os.Stdout, path, listMWAAEnvironments); err != nil {
			fmt.Fprintf(os.Stderr, "airpeek: %v\n", err)
			return 1
		}
		return 0
	}

	cfg, err := fromConfigFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "airpeek: %v\n", err)
		return 1
	}
	cfg.Env = *env
	if _, err := tea.NewProgram(tui.New(cfg), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "airpeek: %v\n", err)
		return 1
	}
	return 0
}

func buildVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}
