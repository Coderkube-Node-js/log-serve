package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type LogRecord struct {
	ID         int64     `json:"id"`
	Level      string    `json:"level"`
	Type       string    `json:"type"`
	Timestamp  time.Time `json:"timestamp"`
	LatencyMS  int       `json:"latency_ms"`
	Route      string    `json:"route"`
	Message    string    `json:"message"`
	Source     string    `json:"source"`
	Server     string    `json:"server"`
	StatusCode int       `json:"status_code"`
	IP         string    `json:"ip"`
	URL        string    `json:"url"`
	Method     string    `json:"method"`
	Raw        string    `json:"raw"`
}

type LogFilter struct {
	Page      int
	Limit     int
	Level     string
	Type      string
	Status    int
	Search    string
	StartDate time.Time
	EndDate   time.Time
	IP        string
	URL       string
	Method    string
	Server    string
	LiveOnly  bool
}

type Stats struct {
	Total    int64 `json:"total"`
	Requests int64 `json:"requests"`
	Warnings int64 `json:"warnings"`
	Errors   int64 `json:"errors"`
}

type FilterMetadata struct {
	Levels  []string `json:"levels"`
	Types   []string `json:"types"`
	Methods []string `json:"methods"`
	Servers []string `json:"servers"`
	IPs     []string `json:"ips"`
	HasData struct {
		Status bool `json:"status"`
		IP     bool `json:"ip"`
		URL    bool `json:"url"`
		Method bool `json:"method"`
		Server bool `json:"server"`
		Date   bool `json:"date"`
	} `json:"has_data"`
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS logs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	level TEXT NOT NULL,
	type TEXT NOT NULL,
	ts DATETIME NOT NULL,
	latency_ms INTEGER NOT NULL DEFAULT 0,
	route TEXT NOT NULL DEFAULT '',
	message TEXT NOT NULL DEFAULT '',
	source TEXT NOT NULL DEFAULT '',
	server TEXT NOT NULL DEFAULT '',
	status_code INTEGER NOT NULL DEFAULT 0,
	ip TEXT NOT NULL DEFAULT '',
	url TEXT NOT NULL DEFAULT '',
	method TEXT NOT NULL DEFAULT '',
	raw TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_logs_ts ON logs(ts DESC);
CREATE INDEX IF NOT EXISTS idx_logs_level ON logs(level);
CREATE INDEX IF NOT EXISTS idx_logs_type ON logs(type);
CREATE INDEX IF NOT EXISTS idx_logs_status ON logs(status_code);
CREATE INDEX IF NOT EXISTS idx_logs_ip ON logs(ip);
CREATE INDEX IF NOT EXISTS idx_logs_url ON logs(url);
CREATE INDEX IF NOT EXISTS idx_logs_server ON logs(server);
CREATE VIRTUAL TABLE IF NOT EXISTS logs_fts USING fts5(message, route, source, ip, url, method, raw, content='logs', content_rowid='id');
CREATE TRIGGER IF NOT EXISTS logs_ai AFTER INSERT ON logs BEGIN
  INSERT INTO logs_fts(rowid, message, route, source, ip, url, method, raw)
  VALUES (new.id, new.message, new.route, new.source, new.ip, new.url, new.method, new.raw);
END;
CREATE TRIGGER IF NOT EXISTS logs_ad AFTER DELETE ON logs BEGIN
  INSERT INTO logs_fts(logs_fts, rowid, message, route, source, ip, url, method, raw)
  VALUES ('delete', old.id, old.message, old.route, old.source, old.ip, old.url, old.method, old.raw);
END;
CREATE TRIGGER IF NOT EXISTS logs_au AFTER UPDATE ON logs BEGIN
  INSERT INTO logs_fts(logs_fts, rowid, message, route, source, ip, url, method, raw)
  VALUES ('delete', old.id, old.message, old.route, old.source, old.ip, old.url, old.method, old.raw);
  INSERT INTO logs_fts(rowid, message, route, source, ip, url, method, raw)
  VALUES (new.id, new.message, new.route, new.source, new.ip, new.url, new.method, new.raw);
END;
`
	_, err := s.db.Exec(schema)
	return err
}

func (s *Store) InsertBatch(ctx context.Context, records []LogRecord) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO logs(level,type,ts,latency_ms,route,message,source,server,status_code,ip,url,method,raw) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, r := range records {
		if _, err := stmt.ExecContext(ctx, r.Level, r.Type, r.Timestamp.UTC(), r.LatencyMS, r.Route, r.Message, r.Source, r.Server, r.StatusCode, r.IP, r.URL, r.Method, r.Raw); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Query(ctx context.Context, f LogFilter) ([]LogRecord, int64, error) {
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.Limit <= 0 {
		f.Limit = 20
	}
	if f.Limit > 1000 {
		f.Limit = 1000
	}
	where := []string{"1=1"}
	args := []any{}
	joinFTS := false
	if f.Level != "" {
		where = append(where, "level = ?")
		args = append(args, strings.ToUpper(f.Level))
	}
	if f.Type != "" {
		where = append(where, "type = ?")
		args = append(args, strings.ToUpper(f.Type))
	}
	if f.Status != 0 {
		where = append(where, "status_code = ?")
		args = append(args, f.Status)
	}
	if !f.StartDate.IsZero() {
		where = append(where, "ts >= ?")
		args = append(args, f.StartDate.UTC())
	}
	if !f.EndDate.IsZero() {
		where = append(where, "ts <= ?")
		args = append(args, f.EndDate.UTC())
	}
	if f.IP != "" {
		where = append(where, "ip LIKE ?")
		args = append(args, like(f.IP))
	}
	if f.URL != "" {
		where = append(where, "url LIKE ?")
		args = append(args, like(f.URL))
	}
	if f.Method != "" {
		where = append(where, "method = ?")
		args = append(args, strings.ToUpper(f.Method))
	}
	if f.Server != "" {
		where = append(where, "server = ?")
		args = append(args, f.Server)
	}
	if f.Search != "" {
		joinFTS = true
		where = append(where, "logs_fts MATCH ?")
		args = append(args, f.Search+"*")
	}

	from := "logs"
	if joinFTS {
		from = "logs JOIN logs_fts ON logs_fts.rowid = logs.id"
	}
	whereSQL := strings.Join(where, " AND ")
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", from, whereSQL)
	var total int64
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	querySQL := fmt.Sprintf(`SELECT logs.id, level, type, ts, latency_ms, route, message, source, server, status_code, ip, url, method, raw
		FROM %s WHERE %s ORDER BY ts DESC LIMIT ? OFFSET ?`, from, whereSQL)
	args = append(args, f.Limit, (f.Page-1)*f.Limit)
	rows, err := s.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []LogRecord
	for rows.Next() {
		var r LogRecord
		if err := rows.Scan(&r.ID, &r.Level, &r.Type, &r.Timestamp, &r.LatencyMS, &r.Route, &r.Message, &r.Source, &r.Server, &r.StatusCode, &r.IP, &r.URL, &r.Method, &r.Raw); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func (s *Store) Export(ctx context.Context, f LogFilter) ([]LogRecord, error) {
	f.Page = 1
	f.Limit = 100000
	rows, _, err := s.Query(ctx, f)
	return rows, err
}

func (s *Store) Stats(ctx context.Context) (Stats, error) {
	var stats Stats
	err := s.db.QueryRowContext(ctx, `
SELECT
	(SELECT COUNT(*) FROM logs),
	(SELECT COUNT(*) FROM logs WHERE type IN ('REQUEST','ACCESS')),
	(SELECT COUNT(*) FROM logs WHERE level = 'WARN'),
	(SELECT COUNT(*) FROM logs WHERE level = 'ERROR')
`).Scan(&stats.Total, &stats.Requests, &stats.Warnings, &stats.Errors)
	return stats, err
}

func (s *Store) DeleteAll(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM logs`)
	return err
}

func (s *Store) DeleteOlderThan(ctx context.Context, before time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM logs WHERE ts < ?`, before.UTC())
	return err
}

func (s *Store) LastN(ctx context.Context, n int) ([]LogRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, level, type, ts, latency_ms, route, message, source, server, status_code, ip, url, method, raw FROM logs ORDER BY ts DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LogRecord
	for rows.Next() {
		var r LogRecord
		if err := rows.Scan(&r.ID, &r.Level, &r.Type, &r.Timestamp, &r.LatencyMS, &r.Route, &r.Message, &r.Source, &r.Server, &r.StatusCode, &r.IP, &r.URL, &r.Method, &r.Raw); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) Metadata(ctx context.Context) (FilterMetadata, error) {
	var meta FilterMetadata
	var err error
	if meta.Levels, err = s.distinctStrings(ctx, `SELECT DISTINCT level FROM logs WHERE level <> '' ORDER BY level`); err != nil {
		return meta, err
	}
	if meta.Types, err = s.distinctStrings(ctx, `SELECT DISTINCT type FROM logs WHERE type <> '' ORDER BY type`); err != nil {
		return meta, err
	}
	if meta.Methods, err = s.distinctStrings(ctx, `SELECT DISTINCT method FROM logs WHERE method <> '' ORDER BY method`); err != nil {
		return meta, err
	}
	if meta.Servers, err = s.distinctStrings(ctx, `SELECT DISTINCT server FROM logs WHERE server <> '' ORDER BY server`); err != nil {
		return meta, err
	}
	if meta.IPs, err = s.distinctStrings(ctx, `SELECT DISTINCT ip FROM logs WHERE ip <> '' ORDER BY ip LIMIT 20`); err != nil {
		return meta, err
	}
	meta.HasData.Status, err = s.hasAny(ctx, `SELECT 1 FROM logs WHERE status_code <> 0 LIMIT 1`)
	if err != nil {
		return meta, err
	}
	meta.HasData.IP, err = s.hasAny(ctx, `SELECT 1 FROM logs WHERE ip <> '' LIMIT 1`)
	if err != nil {
		return meta, err
	}
	meta.HasData.URL, err = s.hasAny(ctx, `SELECT 1 FROM logs WHERE url <> '' LIMIT 1`)
	if err != nil {
		return meta, err
	}
	meta.HasData.Method, err = s.hasAny(ctx, `SELECT 1 FROM logs WHERE method <> '' LIMIT 1`)
	if err != nil {
		return meta, err
	}
	meta.HasData.Server, err = s.hasAny(ctx, `SELECT 1 FROM logs WHERE server <> '' LIMIT 1`)
	if err != nil {
		return meta, err
	}
	meta.HasData.Date, err = s.hasAny(ctx, `SELECT 1 FROM logs LIMIT 1`)
	if err != nil {
		return meta, err
	}
	return meta, nil
}

func MarshalJSONLines(records []LogRecord) ([]byte, error) {
	return json.MarshalIndent(records, "", "  ")
}

func like(value string) string {
	return "%" + strings.ReplaceAll(value, "%", "") + "%"
}

func (s *Store) distinctStrings(ctx context.Context, query string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		if value != "" {
			out = append(out, value)
		}
	}
	return out, rows.Err()
}

func (s *Store) hasAny(ctx context.Context, query string) (bool, error) {
	var value int
	err := s.db.QueryRowContext(ctx, query).Scan(&value)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
