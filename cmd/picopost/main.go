// Command picopost is the PicoPost server binary.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/matthewsawatzky/PicoPost/internal/config"
	"github.com/matthewsawatzky/PicoPost/internal/database"
	"github.com/matthewsawatzky/PicoPost/internal/server"
)

const version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "picopost:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}

	switch args[0] {
	case "serve":
		return cmdServe(args[1:])
	case "version":
		fmt.Println("picopost " + version)
		return nil
	case "check":
		return cmdCheck(args[1:])
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q (try \"picopost help\")", args[0])
	}
}

func usage() {
	fmt.Print(`PicoPost - tiny text infrastructure for the web

Usage:
  picopost serve [--config <path>]   run the HTTP server
  picopost check [--config <path>]    validate config and database, then exit
  picopost version                   print the version
  picopost help                      show this help

The default config path is ./picopost.toml.
`)
}

func configPath(flagSet *flag.FlagSet) (string, error) {
	path := flagSet.Lookup("config").Value.String()
	if path == "" {
		path = "picopost.toml"
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	fs.String("config", "", "path to config file (default ./picopost.toml)")
	fs.Parse(args)

	path, err := configPath(fs)
	if err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	log.Info("picopost starting", "version", version, "config", path, "database", cfg.Storage.Database)

	db, err := database.Open(cfg.Storage.Database)
	if err != nil {
		return err
	}
	defer db.Close()

	srv := server.New(cfg, db, log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve() }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		log.Info("shutdown complete")
		return nil
	}
}

func cmdCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	fs.String("config", "", "path to config file (default ./picopost.toml)")
	fs.Parse(args)

	path, err := configPath(fs)
	if err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	fmt.Printf("config: OK (%s)\n", path)

	db, err := database.Open(cfg.Storage.Database)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer db.Close()

	version, err := db.SchemaVersion()
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	posts, err := db.CountPosts()
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	idents, err := db.CountIdentities()
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	fmt.Printf("database: OK (%s, schema v%d, %d posts, %d identities)\n", cfg.Storage.Database, version, posts, idents)
	return nil
}
