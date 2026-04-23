package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ashraf/log-serve/internal/database"
	webassets "github.com/ashraf/log-serve/web"
)

type Dependencies struct {
	ServerName        string
	Store             *database.Store
	StartedAt         time.Time
	AuthEnabled       bool
	Username          string
	Password          string
	RequestsPerMinute int
	Burst             int
}

type API struct {
	deps     Dependencies
	upgrader websocket.Upgrader
	hub      *Hub
	limiter  *Limiter
}

type Hub struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]struct{}
}

type Limiter struct {
	mu      sync.Mutex
	clients map[string]*tokenBucket
	rate    float64
	burst   float64
}

type tokenBucket struct {
	tokens float64
	last   time.Time
}

func New(deps Dependencies) *API {
	return &API{
		deps: deps,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		hub: &Hub{clients: make(map[*websocket.Conn]struct{})},
		limiter: &Limiter{
			clients: make(map[string]*tokenBucket),
			rate:    float64(deps.RequestsPerMinute) / 60.0,
			burst:   float64(deps.Burst),
		},
	}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.withMiddlewares(a.handleIndex))
	mux.HandleFunc("/styles.css", a.withMiddlewares(a.handleAsset("text/css; charset=utf-8", "styles.css")))
	mux.HandleFunc("/app.js", a.withMiddlewares(a.handleAsset("application/javascript; charset=utf-8", "app.js")))
	mux.HandleFunc("/api/health", a.withMiddlewares(a.handleHealth))
	mux.HandleFunc("/api/logs", a.withMiddlewares(a.handleLogs))
	mux.HandleFunc("/api/logs/export", a.withMiddlewares(a.handleExport))
	mux.HandleFunc("/api/logs/live", a.withMiddlewares(a.handleLive))
	mux.HandleFunc("/api/stats", a.withMiddlewares(a.handleStats))
	mux.HandleFunc("/api/meta", a.withMiddlewares(a.handleMeta))
	return mux
}

func (a *API) Broadcast(rec database.LogRecord) {
	a.hub.mu.Lock()
	defer a.hub.mu.Unlock()
	for c := range a.hub.clients {
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := c.WriteJSON(rec); err != nil {
			_ = c.Close()
			delete(a.hub.clients, c)
		}
	}
}

func (a *API) withMiddlewares(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.limiter.Allow(clientIP(r)) {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		if a.deps.AuthEnabled {
			user, pass, ok := r.BasicAuth()
			if !ok || user != a.deps.Username || pass != a.deps.Password {
				w.Header().Set("WWW-Authenticate", `Basic realm="server-logger"`)
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

func (a *API) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, err := webassets.Assets.ReadFile("index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (a *API) handleAsset(contentType, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := webassets.Assets.ReadFile(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(data)
	}
}

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"server": a.deps.ServerName,
		"uptime": int(time.Since(a.deps.StartedAt).Seconds()),
	})
}

func (a *API) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := a.deps.Store.Stats(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (a *API) handleMeta(w http.ResponseWriter, r *http.Request) {
	meta, err := a.deps.Store.Metadata(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"server":  a.deps.ServerName,
		"filters": meta,
	})
}

func (a *API) handleLogs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		filter := parseFilter(r)
		logs, total, err := a.deps.Store.Query(r.Context(), filter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items": logs,
			"total": total,
			"page":  filter.Page,
			"limit": filter.Limit,
		})
	case http.MethodDelete:
		if err := a.deps.Store.DeleteAll(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *API) handleExport(w http.ResponseWriter, r *http.Request) {
	filter := parseFilter(r)
	rows, err := a.deps.Store.Export(r.Context(), filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "" {
		format = "json"
	}
	switch format {
	case "json":
		w.Header().Set("Content-Disposition", `attachment; filename="logs.json"`)
		writeJSON(w, http.StatusOK, rows)
	case "txt":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="logs.txt"`)
		for _, row := range rows {
			_, _ = fmt.Fprintf(w, "[%s] [%s] [%s] %s\n", row.Timestamp.Format(time.RFC3339), row.Level, row.Type, row.Raw)
		}
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="logs.csv"`)
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"id", "level", "type", "timestamp", "latency_ms", "route", "message", "source", "server", "status_code", "ip", "url", "method"})
		for _, row := range rows {
			_ = cw.Write([]string{
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
		cw.Flush()
	default:
		http.Error(w, "unsupported export format", http.StatusBadRequest)
	}
}

func (a *API) handleLive(w http.ResponseWriter, r *http.Request) {
	conn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	a.hub.mu.Lock()
	a.hub.clients[conn] = struct{}{}
	a.hub.mu.Unlock()

	last, _ := a.deps.Store.LastN(context.Background(), 50)
	for i := len(last) - 1; i >= 0; i-- {
		_ = conn.WriteJSON(last[i])
	}

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
	a.hub.mu.Lock()
	delete(a.hub.clients, conn)
	a.hub.mu.Unlock()
	_ = conn.Close()
}

func parseFilter(r *http.Request) database.LogFilter {
	q := r.URL.Query()
	filter := database.LogFilter{
		Page:   atoiDefault(q.Get("page"), 1),
		Limit:  atoiDefault(q.Get("limit"), 20),
		Level:  q.Get("level"),
		Type:   q.Get("type"),
		Status: atoiDefault(q.Get("status"), 0),
		Search: strings.TrimSpace(q.Get("search")),
		IP:     q.Get("ip"),
		URL:    q.Get("url"),
		Method: q.Get("method"),
		Server: q.Get("server"),
	}
	if v := q.Get("start_date"); v != "" {
		filter.StartDate = parseDate(v)
	}
	if v := q.Get("end_date"); v != "" {
		filter.EndDate = parseDate(v)
	}
	return filter
}

func parseDate(v string) time.Time {
	layouts := []string{time.RFC3339, "2006-01-02"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, v); err == nil {
			return t
		}
	}
	return time.Time{}
}

func atoiDefault(v string, fallback int) int {
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func writeJSON(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (l *Limiter) Allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.clients[key]
	if b == nil {
		l.clients[key] = &tokenBucket{tokens: l.burst - 1, last: now}
		return true
	}
	elapsed := now.Sub(b.last).Seconds()
	b.tokens = min(l.burst, b.tokens+elapsed*l.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
