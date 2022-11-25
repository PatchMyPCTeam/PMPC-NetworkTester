package goCMTrace

import (
	"log"
	"os"
	"runtime"
	"strconv"
	"time"
)

type LogEntry struct {
	Message   string
	time      string
	date      string
	Component string
	context   string
	//The state must be 1, 2 or 3. - where 1 = normal 2 = warning 3 = Error
	State  int
	Thread string
	File   string
}

func LogData(logLine LogEntry) error {
	logFile, err := os.OpenFile(logLine.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
	}
	if logLine.State != 2 && logLine.State != 3 {
		logLine.State = 1
	}
	//Get Caller Component
	pc, _, _, ok := runtime.Caller(1)
	details := runtime.FuncForPC(pc)
	if ok && details != nil {
		callingFile, callingLine := details.FileLine(pc)
		logLine.Component = details.Name()
		logLine.context = callingFile
		logLine.Thread = strconv.Itoa(callingLine)
	}
	//GetDate Time Info
	date := time.Now()
	logLine.date = date.Format("01-02-2006")
	logLine.time = date.Format("15:04:05.999999")

	//serialize log line
	info := "<![LOG[" + logLine.Message + "]LOG]!><time=\"" + logLine.time + "\" date=\"" + logLine.date + "\" component=\"" + logLine.Component + "\" context=\"" + logLine.context + "\" type=\"" + strconv.Itoa(logLine.State) + "\" thread=\"" + logLine.Thread + "\" file=" + logLine.File + "\"\">\n"
	defer logFile.Close()
	if _, err := logFile.WriteString(info); err != nil {
		log.Println(err)
	}
	return err
}
