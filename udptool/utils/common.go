package utils

import (
	"fmt"
	"time"
)

const (
	MagicHeader = "udpZ"
	HeaderLen   = len(MagicHeader)
)

func PrintT(format string, a ...interface{}) {
	fmtStr := fmt.Sprintf(format, a...)
	fmt.Printf("%s %s", time.Now().Format("2006-01-02 15:04:05"), fmtStr)
}
