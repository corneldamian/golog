package golog

import (
	"fmt"
	"io"
	"log"
	"runtime"
	"strconv"
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
	Level        LogLevel
	Verbosity    LogVerbosity
	GoLogLevel   LogLevel
	Prefix string
	FileDepth int
	HeaderWriter func(io.Writer)
	FooterWriter func(io.Writer)

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

	for _, logger := range registeredLoggers {
		close(logger.manager.C)
	}

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

//will create a new logger instance (not go routine safe)
func NewLogger(name, filepath string, fileRotateSize, queueSize int) *Logger {
	if _, found := registeredLoggers[name]; found {
		panic("Logger " + name + " already registered")
	}

	l := &Logger{
		Level:        INFO,
		Verbosity:    LDefault,
		GoLogLevel:   INFO,
		Prefix: "",
		FileDepth: 2,
		HeaderWriter: defaultHeaderWriter,
		FooterWriter: defaultFooterWriter,

		manager: newManager(filepath, fileRotateSize, queueSize),
	}
	l.manager.logger = l

	registeredLoggers[name] = l

	return l
}

//get an existing logger
func GetLogger(name string) *Logger {
	if logger, found := registeredLoggers[name]; found {
		return logger
	}

	panic("Logger " + name + " not registered")
}

func (l *Logger) Debug(v ...interface{}) {
	if l.Level < DEBUG {
		return
	}

	l.manager.C <- l.createMessage(l.FileDepth, DEBUG, v...)
}

func (l *Logger) Info(v ...interface{}) {
	if l.Level < INFO {
		return
	}

	l.manager.C <- l.createMessage(l.FileDepth, INFO, v...)
}

func (l *Logger) Warning(v ...interface{}) {
	if l.Level < WARNING {
		return
	}

	l.manager.C <- l.createMessage(l.FileDepth, WARNING, v...)
}

func (l *Logger) Error(v ...interface{}) {
	if l.Level < ERROR {
		return
	}

	l.manager.C <- l.createMessage(l.FileDepth, ERROR, v...)
}

//create another logger that will add a prefix
func (l *Logger) WithPrefix(prefix string) *Logger{
	return &Logger{
		Level:        l.Level,
		Verbosity:    l.Verbosity,
		GoLogLevel:   l.GoLogLevel,
		Prefix: prefix,
		HeaderWriter: l.HeaderWriter,
		FooterWriter: l.FooterWriter,
		FileDepth: l.FileDepth,

		manager: l.manager,
	}
}

//set the depth of the file/line log (this is if you want to create wrapper over it)
func (l *Logger) SetFileDepth(depth int) {
	l.FileDepth = depth
}

func (l *Logger) Write(p []byte) (n int, err error) {
	if l.Level < l.GoLogLevel {
		return
	}

	l.manager.C <- l.createMessage(l.FileDepth + 2, l.GoLogLevel, string(p[0:len(p)-1]))

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
	msg.vebosity = l.Verbosity
	msg.prefix = &l.Prefix

	if l.Verbosity&LFile != 0 {
		_, file, line, ok := runtime.Caller(calldepth)
		if !ok {
			file = "???"
			line = 0
		}

		if l.Verbosity&LFileLong == 0 {
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
	prefix *string
	date         time.Time
	level        LogLevel
	vebosity     LogVerbosity
	message      []interface{}
	callLocation string
}
