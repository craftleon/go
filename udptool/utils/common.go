package utils

import (
	"fmt"
	"io"
	"time"
)

func PrintTee(w io.Writer, format string, a ...interface{}) {
	fmtStr := time.Now().Format("2006-01-02 15:04:05") + " " + fmt.Sprintf(format, a...)
	fmt.Print(fmtStr)
	if w != nil {
		w.Write([]byte(fmtStr))
	}
}
