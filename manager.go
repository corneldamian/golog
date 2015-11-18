package golog

import (
	"fmt"
	"io"
	"os"
	"time"
)

//how much to write in stderr before retry to write in file
const tempStderrWriteSize = 500 * 1024

func newManager(fileName string, config *LoggerConfig) *logmanager {
	manage := &logmanager{
		C:              make(chan *message, config.MessageQueueSize),
		fileName:       fileName,
		fileRotateSize: config.FileRotateSize,
		config:         config,
	}

	manage.start()

	return manage
}

type logmanager struct {
	C       chan *message

	config          *LoggerConfig
	fileName        string
	fileRotateSize  int
	currentFileSize int
	currentFile     io.Writer
}

func (l *logmanager) start() {
	go func() {
		l.newFile()

		defer l.closeFile()

		var buf []byte

		for {
			select {
			case m, _ := <-l.C:
				buf = buf[:0]
				l.formatHeader(&buf, m)
				l.write(&buf)
			}
		}
	}()
}

func (l *logmanager) formatHeader(buf *[]byte, msg *message) {
	if l.config.Verbosity&LUTC != 0 {
		msg.date = msg.date.UTC()
	}

	if l.config.Verbosity&(LDate|LTime|LMicroseconds) != 0 {
		if l.config.Verbosity&LDate != 0 {
			year, month, day := msg.date.Date()
			itoa(buf, year, 4)
			*buf = append(*buf, '/')
			itoa(buf, int(month), 2)
			*buf = append(*buf, '/')
			itoa(buf, day, 2)
			*buf = append(*buf, ' ')
		}
		if l.config.Verbosity&(LTime|LMicroseconds) != 0 {
			hour, min, sec := msg.date.Clock()
			itoa(buf, hour, 2)
			*buf = append(*buf, ':')
			itoa(buf, min, 2)
			*buf = append(*buf, ':')
			itoa(buf, sec, 2)
			if l.config.Verbosity&LMicroseconds != 0 {
				*buf = append(*buf, '.')
				itoa(buf, msg.date.Nanosecond()/1e3, 6)
			}
			*buf = append(*buf, ' ')
		}
	}

	if l.config.Verbosity&LLevel != 0 {
		*buf = append(*buf, msg.level.String()...)
		*buf = append(*buf, ' ')
	}

	if l.config.Verbosity&LFile != 0 {
		*buf = append(*buf, '[')
		*buf = append(*buf, msg.callLocation...)
		*buf = append(*buf, "] "...)
	}

	if l.config.Prefix != "" {
		*buf = append(*buf, '[')
		*buf = append(*buf, l.config.Prefix...)
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

		if l.config.Verbosity&LHeaderFooter != 0 {
			l.config.FooterWriter(fc)
		}

		fc.Close()
	}
}

func (l *logmanager) newFile() {
	file := l.fileName + ".log"
	l.currentFileSize = 0

	if l.currentFile != os.Stderr {
		if err := l.rename(file); err != nil {
			if err == os.ErrExist {
				l.fileRotateSize += l.config.FileRotateSize / 20
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

	if l.config.Verbosity&LHeaderFooter != 0 {
		l.config.HeaderWriter(ff)
	}

	l.fileRotateSize = l.config.FileRotateSize
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
		if l.config.Verbosity&LUTC != 0 {
			t = t.UTC()
		}

		renameToFile = fmt.Sprintf("%s-%s.log", l.fileName, t.Format("01-02-2006_15-04-05"))

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
