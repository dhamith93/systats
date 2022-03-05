package logger

import (
	"log"
)

// Log logs to given log file
func Log(prefix string, msg string) {
	log.Println(prefix + " " + msg)
}
