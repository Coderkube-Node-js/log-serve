package server

import (
	"io"
	"log"
	"os"
)

func NewAppLogger() (*log.Logger, io.Closer, error) {
	f, err := os.OpenFile("/var/log/server-logger/app.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	return log.New(f, "", log.LstdFlags|log.LUTC|log.Lmicroseconds), f, nil
}
