# golog

Very fast go logger service with rotate support

No mutex used, all logs are written on a separate routine using channels, because of this you need to call Stop to be sure that all logs are written to file and file closed.

In case of error on file create/rename will write to stderr and try from time to time to create file

[![GoDoc](https://godoc.org/github.com/corneldamian/golog?status.svg)](https://godoc.org/github.com/corneldamian/golog)
[![Build Status](https://travis-ci.org/corneldamian/golog.svg?branch=master)](https://travis-ci.org/corneldamian/golog)

Example

```
package main
import (
    "github.com/corneldamian/golog"
    "fmt"
   )

func main() {
    log := golog.NewLogger("log", "logfile", 10*1024*1024, 0)
    log.Info("Test %s", "Today")
    
    //if you need a go standard logger for some libs you can use GetGoLogger
    lgo := log.GetGoLogger()
    lgo.Printf("go logger test %s", "something")
    
    if err:=golog.Stop(2 * time.Second); err != nil {
        fmt.Println("ERROR:", err) 
    }
}
```