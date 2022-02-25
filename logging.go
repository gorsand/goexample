package main

import (
	"log"
	"os"
)

var _logInfo = log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
var _logWarn = log.New(os.Stdout, "WARN\t", log.Ldate|log.Ltime|log.Lshortfile)
var _logError = log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

func LogInfo(msg string, args ...interface{}) {
	_logInfo.Printf(msg, args...)
}

func LogWarn(msg string, args ...interface{}) {
	_logWarn.Printf(msg, args...)
}

func LogError(msg string, args ...interface{}) {
	_logError.Printf(msg, args...)
}

func LogFatal(msg string, args ...interface{}) {
	_logError.Fatalf(msg, args...)
}
