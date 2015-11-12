package golog

import (
	"testing"
	"time"
)

func TestLogger(t *testing.T) {
	l := NewLogger("test1", "test1", 1000, 0)
	l.Verbosity = LDefault | LFile | LMicroseconds | LFileLong
	l.Info("log line test")

	lgo := l.GetGoLogger()
	lgo.Printf("go logger test %s", "something")

	if err := Stop(2 * time.Second); err != nil {
		t.Fatal(err)
	}
}
