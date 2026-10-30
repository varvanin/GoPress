package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"gopress/internal/backup"
	"gopress/internal/httpui"
	"gopress/internal/service"
	"gopress/internal/store"
	"gopress/internal/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: gopress serve|version|backup create|verify|restore")
	}
	if args[0] == "version" {
		fmt.Println("GoPress " + version.Current)
		return nil
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	data := flags.String("data", env("GOPRESS_DATA", "./data"), "local persistent data directory")
	addr := flags.String("addr", env("GOPRESS_ADDR", "127.0.0.1:8080"), "listen address")
	installed := flags.Bool("installed", false, "require a completed site installation for healthcheck")
	email := flags.String("email", "", "account email for password reset")
	passwordStdin := flags.Bool("password-stdin", false, "read the new password from stdin")
	output := flags.String("output", "", "new backup directory outside data")
	source := flags.String("source", "", "existing backup directory")
	action := ""
	rest := args[1:]
	if args[0] == "backup" || args[0] == "admin" || args[0] == "search" || args[0] == "maintenance" {
		if len(rest) == 0 {
			return errors.New("backup create|verify|restore")
		}
		action = rest[0]
		rest = rest[1:]
	}
	if err := flags.Parse(rest); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	switch args[0] {
	case "backup":
		switch action {
		case "create":
			if *output == "" {
				return errors.New("--output is required")
			}
			return backup.Create(*data, *output)
		case "verify":
			if *source == "" {
				return errors.New("--source is required")
			}
			return backup.Verify(*source)
		case "recover":
			if *source == "" {
				return errors.New("--source is required")
			}
			return backup.Recover(*source, *data)
		case "restore":
			if *source == "" {
				return errors.New("--source is required")
			}
			return backup.Restore(*source, *data)
		default:
			return errors.New("unknown backup command")
		}
	case "maintenance":
		if action != "enable" && action != "disable" {
			return errors.New("maintenance enable|disable")
		}
		if _, err := os.Stat(filepath.Join(*data, "site.db")); err != nil {
			return err
		}
		if action == "enable" {
			return os.WriteFile(filepath.Join(*data, "maintenance"), []byte(time.Now().UTC().Format(time.RFC3339)), 0600)
		}
		err := os.Remove(filepath.Join(*data, "maintenance"))
		if os.IsNotExist(err) {
			return nil
		}
		return err
	case "doctor", "migrate", "admin", "search":
		if _, err := os.Stat(filepath.Join(*data, "site.db")); err != nil {
			return err
		}
		s, err := store.Open(*data)
		if err != nil {
			return err
		}
		defer s.Close()
		v, err := service.New(s)
		if err != nil {
			return err
		}
		switch args[0] {
		case "doctor":
			d, err := v.Diagnose()
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(d)
		case "migrate":
			fmt.Println("Migrations complete")
			return nil
		case "search":
			if action != "rebuild" {
				return errors.New("search rebuild")
			}
			return s.RebuildSearch()
		case "admin":
			if action != "reset-password" || !*passwordStdin || *email == "" {
				return errors.New("admin reset-password --email owner@example.com --password-stdin")
			}
			raw, err := io.ReadAll(io.LimitReader(os.Stdin, 74))
			if err != nil {
				return err
			}
			return v.AdminResetPassword(context.Background(), strings.ToLower(strings.TrimSpace(*email)), strings.TrimRight(string(raw), "\r\n"))
		}
		return nil
	case "healthcheck":
		client := &http.Client{Timeout: 3 * time.Second}
		path := "/readyz"
		if *installed {
			path += "?installed=1"
		}
		response, err := client.Get("http://" + *addr + path)
		if err != nil {
			return errors.New("readiness check failed")
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			return errors.New("readiness check failed")
		}
		return nil
	case "serve":
		s, err := store.Open(*data)
		if err != nil {
			return err
		}
		defer s.Close()
		v, err := service.New(s)
		if err != nil {
			return err
		}
		// The one-time token is deliberately emitted only to the local process console.
		if v.InstallToken != "" {
			fmt.Fprintf(os.Stderr, "GoPress installation token (enter in /install): %s\n", v.InstallToken)
		}
		app := httpui.New(v)
		if err = app.ConfigureProxies(os.Getenv("GOPRESS_TRUSTED_PROXIES")); err != nil {
			return err
		}
		srv := &http.Server{Addr: *addr, Handler: app.Echo, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
		listener, err := net.Listen("tcp", *addr)
		if err != nil {
			return err
		}
		defer listener.Close()
		if err = v.CompleteUpdate(); err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		schedulerDone := make(chan struct{})
		go func() { defer close(schedulerDone); v.RunScheduler(ctx) }()
		mailDone := make(chan struct{})
		go func() { defer close(mailDone); v.RunMailWorker(ctx) }()
		defer func() { v.StopUpdates(); stop(); <-schedulerDone; <-mailDone; v.Background.Wait() }()
		done := make(chan error, 1)
		go func() { slog.Info("GoPress listening", "address", *addr); done <- srv.Serve(listener) }()
		select {
		case err = <-done:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-ctx.Done():
			v.StopUpdates()
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err = srv.Shutdown(shutdown); err != nil {
				_ = srv.Close()
				return err
			}
			return nil
		}
	default:
		return errors.New("unknown command")
	}
}
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
