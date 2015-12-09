package golog

import (
	"fmt"
	"io"
	"log"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type LogLevel int

const (
	ERROR LogLevel = iota
	WARNING
	INFO
	DEBUG
)

const (
	lerror   = "ERROR"
	lwarning = "WARNING"
	linfo    = "INFO"
	ldebug   = "DEBUG"
	lunknown = "UNKNOWN"
)

//LogLevel to string
func (ll LogLevel) String() string {
	switch ll {
	case ERROR:
		return lerror
	case WARNING:
		return lwarning
	case INFO:
		return linfo
	case DEBUG:
		return ldebug
	}

	return lunknown
}

//string to LogLevel (if unknown will return INFO)
func ToLogLevel(loglevel string) LogLevel {
	loglevel = strings.ToUpper(loglevel)

	switch loglevel {
	case lerror:
		return ERROR
	case lwarning:
		return WARNING
	case linfo:
		return INFO
	case ldebug:
		return DEBUG
	}

	return INFO
}

type LogVerbosity int

const (
	LDate         LogVerbosity = 1 << iota //log the date
	LTime                                  //log the time
	LMicroseconds                          //time resolution
	LUTC                                   //log the date/time in utc not the local timezone
	LFile                                  //log the filename and line where the log was called
	LFileLong                              //log the full path of the log (LFile must be set)
	LLevel                                 //log the level
	LHeaderFooter                          //log header footer on log file
	LDefault      = LDate | LTime | LLevel
)

var registeredLoggers = make(map[string]*Logger)

var defaultHeaderWriter = func(w io.Writer) {
	fmt.Fprintf(w, "#Start log at: %s\n", time.Now().String())
}

var defaultFooterWriter = func(w io.Writer) {
	fmt.Fprintf(w, "#Stop log at: %s\n", time.Now().String())
}

type Logger struct {
	level      LogLevel
	verbosity  LogVerbosity
	goLogLevel LogLevel
	fileDepth  int

	manager  *logmanager
	gologger *log.Logger
}

// stop all logger services
// will wait the timeout for the logger service to finish writing all the messages from the queue
//
// don't call any log after this, will panic
func Stop(timeout time.Duration) error {
	checkClients := time.Tick(100 * time.Millisecond)
	timeoutTime := time.NewTimer(timeout)

	for {
		select {
		case <-checkClients:
			hasInQueue := false
			for _, logger := range registeredLoggers {
				if len(logger.manager.C) > 0 {
					hasInQueue = true
					break
				}
			}
			if !hasInQueue {
				return nil
			}
		case <-timeoutTime.C:
			message := ""
			for name, logger := range registeredLoggers {
				message = fmt.Sprintf("%s queue: %s size: %d", message, name, len(logger.manager.C))
			}

			return fmt.Errorf("Logger was stopped forced after timeout %s with not logged: %s", timeout, message)
		}
	}
}

type LoggerConfig struct {
	FileRotateSize   int // default 16MB
	MessageQueueSize int
	Level            LogLevel
	Verbosity        LogVerbosity
	GoLogLevel       LogLevel
	Prefix           string
	FileDepth        int
	HeaderWriter     func(io.Writer)
	FooterWriter     func(io.Writer)
}

//will create a new logger instance (not go routine safe)
func NewLogger(loggerName, fileName string, config *LoggerConfig) *Logger {
	if _, found := registeredLoggers[loggerName]; found {
		panic("Logger " + loggerName + " already registered")
	}

	if config == nil {
		config = &LoggerConfig{
			FileRotateSize:   (2 << 23), /*16MB*/
			MessageQueueSize: 50000,
			Level:            INFO,
			Verbosity:        LDefault,
			GoLogLevel:       INFO,
			Prefix:           "",
			FileDepth:        2,
			HeaderWriter:     defaultHeaderWriter,
			FooterWriter:     defaultFooterWriter,
		}
	} else {
		if config.HeaderWriter == nil {
			config.HeaderWriter = defaultHeaderWriter
		}

		if config.FooterWriter == nil {
			config.FooterWriter = defaultFooterWriter
		}
	}

	if config.MessageQueueSize == 0 {
		config.MessageQueueSize = 50000
	}

	l := &Logger{
		level:      config.Level,
		verbosity:  config.Verbosity,
		goLogLevel: config.GoLogLevel,
		fileDepth:  config.FileDepth,
		manager:    newManager(fileName, config),
	}

	registeredLoggers[loggerName] = l

	return l
}

//get an existing logger
func GetLogger(loggerName string) *Logger {
	if logger, found := registeredLoggers[loggerName]; found {
		return logger
	}

	panic("Logger " + loggerName + " not registered")
}

func (l *Logger) Debug(v ...interface{}) {
	if l.level < DEBUG {
		return
	}

	l.manager.C <- l.createMessage(l.fileDepth, DEBUG, v...)
}

func (l *Logger) Info(v ...interface{}) {
	if l.level < INFO {
		return
	}

	l.manager.C <- l.createMessage(l.fileDepth, INFO, v...)
}

func (l *Logger) Warning(v ...interface{}) {
	if l.level < WARNING {
		return
	}

	l.manager.C <- l.createMessage(l.fileDepth, WARNING, v...)
}

func (l *Logger) Error(v ...interface{}) {
	if l.level < ERROR {
		return
	}

	l.manager.C <- l.createMessage(l.fileDepth, ERROR, v...)
}

func (l *Logger) Debugf(fmt string, v ...interface{}) {
	if l.level < DEBUG {
		return
	}

	v = append(v, v[0])
	v[0] = fmt

	l.manager.C <- l.createMessage(l.fileDepth+1, DEBUG, v...)
}

func (l *Logger) Infof(fmt string, v ...interface{}) {
	if l.level < INFO {
		return
	}

	v = append(v, v[0])
	v[0] = fmt

	l.manager.C <- l.createMessage(l.fileDepth+1, INFO, v...)
}

func (l *Logger) Warningf(fmt string, v ...interface{}) {
	if l.level < WARNING {
		return
	}

	v = append(v, v[0])
	v[0] = fmt

	l.manager.C <- l.createMessage(l.fileDepth+1, WARNING, v...)
}

func (l *Logger) Errorf(fmt string, v ...interface{}) {
	if l.level < ERROR {
		return
	}

	v = append(v, v[0])
	v[0] = fmt

	l.manager.C <- l.createMessage(l.fileDepth+1, ERROR, v...)
}

func (l *Logger) Write(p []byte) (n int, err error) {
	if l.level < l.goLogLevel {
		return
	}

	l.manager.C <- l.createMessage(l.fileDepth+2, l.goLogLevel, string(p[0:len(p)-1]))

	return len(p), nil
}

func (l *Logger) GetGoLogger() *log.Logger {
	if l.gologger == nil {
		l.gologger = log.New(l, "", 0)
	}

	return l.gologger
}

func (l *Logger) createMessage(calldepth int, level LogLevel, v ...interface{}) *message {
	msg := &message{}

	msg.date = time.Now()
	msg.message = v
	msg.level = level

	if l.verbosity&LFile != 0 {
		_, file, line, ok := runtime.Caller(calldepth)
		if !ok {
			file = "???"
			line = 0
		}

		if l.verbosity&LFileLong == 0 {
			short := file
			for i := len(file) - 1; i > 0; i-- {
				if file[i] == '/' {
					short = file[i+1:]
					break
				}
			}
			file = short
		}

		msg.callLocation = file + ":" + strconv.Itoa(line)
	}

	return msg
}

type message struct {
	date         time.Time
	level        LogLevel
	message      []interface{}
	callLocation string
}
