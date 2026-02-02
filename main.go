package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k3air/internal/config"
	"k3air/internal/install"
	"k3air/internal/version"
)

// timeFormat is the global time format for logs
const timeFormat = "2006-01-02 15:04:05"

// textHandler is a custom slog.Handler that formats logs with custom time format
type textHandler struct {
	writer  io.Writer
	level   slog.Level
	enabled func(context.Context, slog.Level) bool
}

func newTextHandler(w io.Writer, level slog.Level) *textHandler {
	return &textHandler{
		writer: w,
		level:  level,
		enabled: func(_ context.Context, l slog.Level) bool {
			return l >= level
		},
	}
}

func (h *textHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.enabled(ctx, level)
}

func (h *textHandler) Handle(ctx context.Context, r slog.Record) error {
	// Build the log line with custom time format
	var sb strings.Builder
	var t time.Time = r.Time
	ts := t.Format(timeFormat)
	sb.WriteString(ts)
	sb.WriteString(" ")
	sb.WriteString(r.Level.String())
	sb.WriteString(" ")

	// Write message
	sb.WriteString(r.Message)

	// Write attributes
	r.Attrs(func(a slog.Attr) bool {
		sb.WriteString(" ")
		sb.WriteString(a.Key)
		sb.WriteString("=")
		sb.WriteString(a.Value.String())
		return true
	})

	sb.WriteString("\n")

	_, err := h.writer.Write([]byte(sb.String()))
	return err
}

func (h *textHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *textHandler) WithGroup(name string) slog.Handler {
	return h
}

func main() {
	// Global flags
	showVersion := flag.Bool("version", false, "show version information")
	showVersionShort := flag.Bool("v", false, "show version information (short)")

	// Parse global flags
	flag.Parse()

	// Handle version flag
	if *showVersion || *showVersionShort {
		printVersion()
		os.Exit(0)
	}

	// Check if a command is provided
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	apply := flag.NewFlagSet("apply", flag.ExitOnError)
	cfgPath := apply.String("f", "init.yaml", "path to config.yaml")
	verbose := apply.Bool("verbose", false, "enable verbose logging")

	init := flag.NewFlagSet("init", flag.ExitOnError)
	switch os.Args[1] {
	case "apply":
		apply.Parse(os.Args[2:])

		// Configure log level based on verbose flag
		logLevel := slog.LevelInfo
		if *verbose {
			logLevel = slog.LevelDebug
		}

		// Use custom handler with formatted time
		handler := newTextHandler(os.Stdout, logLevel)
		logger := slog.New(handler)
		slog.SetDefault(logger)

		cfg, err := config.Load(*cfgPath)
		if err != nil {
			fmt.Println("failed to load config:", err)
			os.Exit(1)
		}
		slog.Info("cluster config", "pod cidr", cfg.Cluster.ClusterCidr, "service cidr", cfg.Cluster.ServiceCidr)
		assetsDir := filepath.Join("assets")
		inst, err := install.NewInstaller(cfg, assetsDir, *verbose)
		if err != nil {
			slog.Error("failed to create installer", "error", err)
			os.Exit(1)
		}
		defer func() {
			if err := inst.Cleanup(); err != nil {
				slog.Warn("cleanup failed", "error", err)
			}
		}()
		if err := inst.Apply(); err != nil {
			slog.Error("apply failed", "error", err)
			os.Exit(1)
		}
		fmt.Println("apply completed")
	case "init":
		init.Parse(os.Args[2:])
		// Get the template file path from internal/config
		templatePath := filepath.Join("internal", "config", "init.yaml.template")
		out := filepath.Join(".", "init.yaml")
		if _, err := os.Stat(out); err == nil {
			fmt.Println("init.yaml already exists")
			os.Exit(1)
		}
		// Read template
		content, err := os.ReadFile(templatePath)
		if err != nil {
			fmt.Println("failed to read template:", err)
			os.Exit(1)
		}
		// Write to init.yaml
		if err := os.WriteFile(out, content, 0644); err != nil {
			fmt.Println("failed to write init.yaml:", err)
			os.Exit(1)
		}
		fmt.Println("created init.yaml ✅，please edit it and run k3air apply -f init.yaml")
		os.Exit(0)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("usage:")
	fmt.Println("  k3air apply -f <config path>   Deploy a k3s cluster")
	fmt.Println("  k3air init                     Create a default config.yaml")
	fmt.Println("  k3air --version, -v            Show version information")
}

func printVersion() {
	fmt.Printf("k3air %s\n", version.Version)
	fmt.Printf("  Build time: %s\n", version.BuildTime)
	fmt.Printf("  Git commit: %s\n", version.GitCommit)
}
