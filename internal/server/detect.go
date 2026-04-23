package server

import (
	"errors"
	"os/exec"
)

type DetectedServer struct {
	Name     string   `json:"name"`
	LogFiles []string `json:"log_files"`
}

func DetectServer(extra []string) (DetectedServer, error) {
	type candidate struct {
		name string
		logs []string
	}
	candidates := []candidate{
		{name: "nginx", logs: []string{"/var/log/nginx/access.log", "/var/log/nginx/error.log"}},
		{name: "apache2", logs: []string{"/var/log/apache2/access.log", "/var/log/apache2/error.log"}},
		{name: "httpd", logs: []string{"/var/log/httpd/access_log", "/var/log/httpd/error_log"}},
	}
	for _, c := range candidates {
		if _, err := exec.LookPath(c.name); err == nil {
			logs := append([]string{}, c.logs...)
			logs = append(logs, extra...)
			return DetectedServer{Name: c.name, LogFiles: logs}, nil
		}
	}
	if len(extra) > 0 {
		return DetectedServer{Name: "custom", LogFiles: extra}, nil
	}
	return DetectedServer{}, errors.New("No supported web server detected.\nInstall nginx, apache2, or httpd.")
}
