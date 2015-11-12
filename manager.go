package golog

import (
	"fmt"
	"io"
	"os"
	"time"
)

//default queue size
const LOGQUEUE = 50000
const tempStderrWriteSize = 500 * 1024

func newManager(filepath string, fileRotateSize, queueSize int) *logmanager {
	if queueSize == 0 {
		queueSize = LOGQUEUE
	}

	manage := &logmanager{
		C:                     make(chan *message, queueSize),
		filepath:              filepath,
		fileRotateSize:        fileRotateSize,
		initialFileRotateSize: fileRotateSize,
	}

	go manage.start()

	return manage
}

type logmanager struct {
	C chan *message

	logger                *Logger
	filepath              string
	fileRotateSize        int
	initialFileRotateSize int
	currentFileSize       int
	currentFile           io.Writer
}

func (l *logmanager) start() {
	go func() {
		l.newFile()

		var buf []byte
		for m := range l.C {
			buf = buf[:0]
			l.formatHeader(&buf, m)
			l.write(&buf)
		}

		l.closeFile()
	}()
}

func (l *logmanager) formatHeader(buf *[]byte, msg *message) {
	if msg.vebosity&LUTC != 0 {
		msg.date = msg.date.UTC()
	}

	if msg.vebosity&(LDate|LTime|LMicroseconds) != 0 {
		if msg.vebosity&LDate != 0 {
			year, month, day := msg.date.Date()
			itoa(buf, year, 4)
			*buf = append(*buf, '/')
			itoa(buf, int(month), 2)
			*buf = append(*buf, '/')
			itoa(buf, day, 2)
			*buf = append(*buf, ' ')
		}
		if msg.vebosity&(LTime|LMicroseconds) != 0 {
			hour, min, sec := msg.date.Clock()
			itoa(buf, hour, 2)
			*buf = append(*buf, ':')
			itoa(buf, min, 2)
			*buf = append(*buf, ':')
			itoa(buf, sec, 2)
			if msg.vebosity&LMicroseconds != 0 {
				*buf = append(*buf, '.')
				itoa(buf, msg.date.Nanosecond()/1e3, 6)
			}
			*buf = append(*buf, ' ')
		}
	}

	if msg.vebosity&LLevel != 0 {
		*buf = append(*buf, msg.level.String()...)
		*buf = append(*buf, ' ')
	}

	if msg.vebosity&LFile != 0 {
		*buf = append(*buf, '[')
		*buf = append(*buf, msg.callLocation...)
		*buf = append(*buf, "] "...)
	}

	if msg.prefix != nil && *msg.prefix != ""{
		*buf = append(*buf, '[')
		*buf = append(*buf, *msg.prefix...)
		*buf = append(*buf, "] "...)
	}


	if len(msg.message) > 1 {
		*buf = append(*buf, fmt.Sprintf(msg.message[0].(string), msg.message[1:]...)...)
	} else {
		*buf = append(*buf, fmt.Sprint(msg.message[0])...)
	}

	if len(*buf) == 0 || (*buf)[len(*buf)-1] != '\n' {
		*buf = append(*buf, '\n')
	}
}

func (l *logmanager) write(p *[]byte) (n int, err error) {
	if l.shouldRotate() || l.currentFile == nil {
		l.newFile()
	}

	n, err = l.currentFile.Write(*p)
	l.currentFileSize += n
	return
}

func (l *logmanager) shouldRotate() bool {
	if l.currentFileSize >= l.fileRotateSize {
		return true
	}

	return false
}

func (l *logmanager) closeFile() {
	if l.currentFile != nil && l.currentFile != os.Stderr {
		fc := l.currentFile.(*os.File)

		if l.logger.Verbosity&LHeaderFooter != 0 {
			l.logger.FooterWriter(fc)
		}
	}
}

func (l *logmanager) newFile() {
	file := l.filepath + ".log"
	l.currentFileSize = 0

	if l.currentFile != os.Stderr {
		if err := l.rename(file); err != nil {
			if err == os.ErrExist {
				l.fileRotateSize += l.initialFileRotateSize / 20
				return
			}
			l.currentFile = os.Stderr
			l.fileRotateSize = tempStderrWriteSize
		}
	}

	ff, err := os.Create(file)
	if err != nil {
		l.currentFile = os.Stderr
		l.fileRotateSize = tempStderrWriteSize
		return
	}

	if l.logger.Verbosity&LHeaderFooter != 0 {
		l.logger.HeaderWriter(ff)
	}

	l.fileRotateSize = l.initialFileRotateSize
	l.currentFile = ff
}

func (l *logmanager) rename(file string) error {
	renameToFile := ""

	f, err := os.Stat(file)
	if err == nil {
		if f.IsDir() {
			return os.ErrInvalid
		}

		t := time.Now()
		if l.logger.Verbosity&LUTC != 0 {
			t = t.UTC()
		}

		renameToFile = fmt.Sprintf("%s-%s.log", l.filepath, t.Format("01-02-2006_15-04-05"))

		_, err := os.Stat(renameToFile)
		if err == nil {
			return os.ErrExist
		}
	}

	l.closeFile()

	if err := os.Rename(file, renameToFile); err != nil {
		return err
	}

	return nil
}

func itoa(buf *[]byte, i int, wid int) {
	// Assemble decimal in reverse order.
	var b [20]byte
	bp := len(b) - 1
	for i >= 10 || wid > 1 {
		wid--
		q := i / 10
		b[bp] = byte('0' + i - q*10)
		bp--
		i = q
	}
	// i < 10
	b[bp] = byte('0' + i)
	*buf = append(*buf, b[bp:]...)
}
