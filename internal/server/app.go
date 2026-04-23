package server

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ashraf/log-serve/internal/api"
	"github.com/ashraf/log-serve/internal/database"
	"github.com/ashraf/log-serve/internal/watcher"
)

func RunCLI(args []string) int {
	switch first(args) {
	case "", "serve":
		if err := runServer(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	case "start", "stop", "restart", "status":
		return runSystemctl(first(args))
	case "logs":
		return showAppLogs()
	case "export":
		return exportCLI(args[1:])
	case "version":
		fmt.Println(Version)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", first(args))
		return 1
	}
}

func runServer() error {
	cfg, err := LoadConfig(DefaultConfigPath)
	if err != nil {
		return err
	}
	if err := EnsureRuntimePaths(cfg); err != nil {
		return err
	}
	appLogger, closer, err := NewAppLogger()
	if err != nil {
		return err
	}
	defer closer.Close()

	detected, err := DetectServer(cfg.Logs.Sources)
	if err != nil {
		appLogger.Println(err.Error())
		return err
	}
	store, err := database.Open(cfg.Database.Path)
	if err != nil {
		return err
	}
	defer store.Close()

	startedAt := time.Now()
	apiServer := api.New(api.Dependencies{
		ServerName:        detected.Name,
		Store:             store,
		StartedAt:         startedAt,
		AuthEnabled:       cfg.Security.AuthEnabled,
		Username:          cfg.Security.Username,
		Password:          cfg.Security.Password,
		RequestsPerMinute: cfg.Security.RateLimit.RequestsPerMinute,
		Burst:             cfg.Security.RateLimit.Burst,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	records := make(chan database.LogRecord, cfg.Logs.BufferSize)
	go ingestLoop(ctx, store, apiServer, records, cfg.Logs.BatchSize, appLogger)
	go cleanupLoop(ctx, store, cfg, appLogger)

	w := watcher.New(detected.Name, detected.LogFiles, time.Duration(cfg.Logs.PollInterval)*time.Second, cfg.Logs.MaxLineBytes, func(rec database.LogRecord) {
		select {
		case records <- rec:
		default:
			appLogger.Printf("dropped log line from %s due to full buffer", rec.Source)
		}
	})
	go w.Run(ctx)

	addr := cfg.Server.Host + ":" + strconv.Itoa(cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           apiServer.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	appLogger.Printf("server-logger %s started on %s (server=%s)", Version, addr, detected.Name)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	if cfg.Security.HTTPS.Enabled {
		if cfg.Security.HTTPS.CertFile == "" || cfg.Security.HTTPS.KeyFile == "" {
			return errors.New("https enabled but cert_file/key_file missing")
		}
		return srv.ListenAndServeTLS(cfg.Security.HTTPS.CertFile, cfg.Security.HTTPS.KeyFile)
	}
	err = srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func ingestLoop(ctx context.Context, store *database.Store, apiServer *api.API, records <-chan database.LogRecord, batchSize int, logger *log.Logger) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	batch := make([]database.LogRecord, 0, batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		toWrite := append([]database.LogRecord(nil), batch...)
		if err := store.InsertBatch(ctx, toWrite); err != nil {
			logger.Printf("insert batch failed: %v", err)
			return
		}
		for _, rec := range toWrite {
			apiServer.Broadcast(rec)
		}
		batch = batch[:0]
	}
	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case rec := <-records:
			batch = append(batch, rec)
			if len(batch) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func cleanupLoop(ctx context.Context, store *database.Store, cfg Config, logger *log.Logger) {
	ticker := time.NewTicker(CleanupInterval(cfg))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			before := time.Now().AddDate(0, 0, -cfg.Logs.RetentionDays)
			if err := store.DeleteOlderThan(ctx, before); err != nil {
				logger.Printf("cleanup failed: %v", err)
			}
		}
	}
}

func runSystemctl(command string) int {
	cmd := exec.Command("systemctl", command, "server-logger")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func showAppLogs() int {
	data, err := os.ReadFile("/var/log/server-logger/app.log")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Print(string(data))
	return 0
}

func exportCLI(args []string) int {
	cfg, err := LoadConfig(DefaultConfigPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	store, err := database.Open(cfg.Database.Path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer store.Close()
	format := "json"
	output := filepath.Join(".", "logs."+format)
	for i := 0; i < len(args); i++ {
		if args[i] == "--format" && i+1 < len(args) {
			format = strings.ToLower(args[i+1])
			output = filepath.Join(".", "logs."+format)
			i++
		}
		if args[i] == "--output" && i+1 < len(args) {
			output = args[i+1]
			i++
		}
	}
	rows, err := store.Export(context.Background(), database.LogFilter{Page: 1, Limit: 100000})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	switch format {
	case "json":
		data, err := database.MarshalJSONLines(rows)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := os.WriteFile(output, data, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	case "txt":
		var b strings.Builder
		for _, row := range rows {
			fmt.Fprintf(&b, "[%s] [%s] [%s] %s\n", row.Timestamp.Format(time.RFC3339), row.Level, row.Type, row.Raw)
		}
		if err := os.WriteFile(output, []byte(b.String()), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	case "csv":
		f, err := os.Create(output)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer f.Close()
		w := csv.NewWriter(f)
		_ = w.Write([]string{"id", "level", "type", "timestamp", "latency_ms", "route", "message", "source", "server", "status_code", "ip", "url", "method"})
		for _, row := range rows {
			_ = w.Write([]string{
				strconv.FormatInt(row.ID, 10),
				row.Level,
				row.Type,
				row.Timestamp.Format(time.RFC3339),
				strconv.Itoa(row.LatencyMS),
				row.Route,
				row.Message,
				row.Source,
				row.Server,
				strconv.Itoa(row.StatusCode),
				row.IP,
				row.URL,
				row.Method,
			})
		}
		w.Flush()
		if err := w.Error(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	default:
		fmt.Fprintln(os.Stderr, "supported formats: json, txt, csv")
		return 1
	}
	fmt.Println(output)
	return 0
}

func first(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}
