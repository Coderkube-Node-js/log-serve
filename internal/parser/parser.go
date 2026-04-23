package parser

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ashraf/log-serve/internal/database"
)

var commonLogPattern = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]+)\] "([A-Z]+) ([^"]+?) [^"]+" (\d{3}) (\S+)`)
var latencyPattern = regexp.MustCompile(`(?:rt|request_time|duration|time)=([0-9.]+)`)

func Parse(serverName, source, line string, now time.Time) database.LogRecord {
	rec := database.LogRecord{
		Level:     "LOG",
		Type:      classifyType(source),
		Timestamp: now.UTC(),
		Message:   truncate(line, 4000),
		Source:    filepath.Base(source),
		Server:    serverName,
		Route:     "-",
		Raw:       truncate(line, 16000),
	}

	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, " error "):
		rec.Level = "ERROR"
		rec.Type = "ERROR"
	case strings.Contains(lower, " warn "), strings.Contains(lower, "[warn]"):
		rec.Level = "WARN"
	case strings.Contains(lower, " info "), strings.Contains(lower, "[info]"):
		rec.Level = "INFO"
	}

	if matches := commonLogPattern.FindStringSubmatch(line); len(matches) == 7 {
		rec.IP = matches[1]
		rec.Method = matches[3]
		rec.URL = matches[4]
		rec.Route = matches[4]
		rec.Type = "ACCESS"
		rec.Level = levelFromStatus(matches[5])
		rec.StatusCode, _ = strconv.Atoi(matches[5])
		rec.Message = summarizeAccess(rec.Method, rec.URL, rec.StatusCode)
		if ts, err := time.Parse("02/Jan/2006:15:04:05 -0700", matches[2]); err == nil {
			rec.Timestamp = ts.UTC()
		}
	}
	if m := latencyPattern.FindStringSubmatch(line); len(m) == 2 {
		if seconds, err := strconv.ParseFloat(m[1], 64); err == nil {
			rec.LatencyMS = int(seconds * 1000)
		}
	}
	if rec.Type == "ERROR" {
		rec.Level = "ERROR"
	}
	return rec
}

func classifyType(source string) string {
	lower := strings.ToLower(source)
	switch {
	case strings.Contains(lower, "access"):
		return "ACCESS"
	case strings.Contains(lower, "error"):
		return "ERROR"
	default:
		return "CONSOLE"
	}
}

func levelFromStatus(code string) string {
	n, err := strconv.Atoi(code)
	if err != nil {
		return "LOG"
	}
	switch {
	case n >= 500:
		return "ERROR"
	case n >= 400:
		return "WARN"
	case n >= 200:
		return "INFO"
	default:
		return "LOG"
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func summarizeAccess(method, url string, status int) string {
	if method == "" && url == "" {
		return ""
	}
	if status == 0 {
		return method + " " + url
	}
	return method + " " + url + " [" + strconv.Itoa(status) + "]"
}
