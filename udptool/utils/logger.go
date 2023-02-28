package utils

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	LogQueueSize = 64
)

type LogWriter struct {
	Prefix string
	Name   string
	msg    chan string
	wg     sync.WaitGroup
}

// async write
func (l *LogWriter) Write(data []byte) (n int, err error) {
	if l == nil {
		return 0, errors.New("log logFileWriter is nil")
	}

	if l.msg == nil {
		l.msg = make(chan string, LogQueueSize)
		l.wg.Add(1)
		go l.writeRoutine()
	}
	l.msg <- string(data)

	return len(data), nil
}

func (l *LogWriter) writeRoutine() {
	defer l.wg.Done()

	for {
		var msgArr []string
		var quit bool

		msg := <-l.msg
		if msg == "quit" {
			quit = true
		} else {
			msgArr = append(msgArr, msg)
		}

	collectMore:
		for {
			select {
			case msg = <-l.msg:
				if msg == "quit" {
					quit = true
				} else {
					msgArr = append(msgArr, msg)
				}
			default:
				break collectMore
			}
		}

		if len(msgArr) > 0 {
			date := time.Now().Format("2006-01-02")
			// O_CREATE create if does not exist，O_APPEND open at the end of file，O_SYNC sync data right into disk for writes
			if len(l.Prefix) > 0 {
				err := os.MkdirAll(l.Prefix, os.ModePerm)
				if err != nil {
					PrintTee(nil, "Cannot create directory for log file: %v. Using current working directory instead.", err)
					l.Prefix = ""
				}
			}
			path := fmt.Sprintf("%s%s-%s.log", l.Prefix, l.Name, date)
			file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE|os.O_SYNC, 0644)
			if err != nil {
				PrintTee(nil, "Log file not opened: %v", err)
				continue
			}

			for _, m := range msgArr {
				_, err := file.Write([]byte(m))
				if err != nil {
					PrintTee(nil, "Log file write failed: %v", err)
					break
				}
			}

			// close after write
			file.Close()
		}

		// quit routine after all valid writes
		if quit {
			return
		}
	}
}

func (l *LogWriter) Close() {
	if l.msg == nil {
		return
	}
	l.msg <- "quit"
	l.wg.Wait()
}
