package utils

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Client struct {
	DestIp   net.IP
	DestPort int
}

func (c *Client) Run() {
	// pick a ephemeral local port
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 0})
	if err != nil {
		PrintT("listen error %v\n", err)
		return
	}

	// retrieve port
	laddr := conn.LocalAddr()
	uaddr, err := net.ResolveUDPAddr(
		laddr.Network(),
		laddr.String(),
	)
	if err != nil {
		PrintT("resolve UDPAddr error %v\n", err)
		return
	}

	PrintT("Connection setup, addr: %s, port %d\n", uaddr.String(), uaddr.Port)

	stop := make(chan struct{})
	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM)
	signal.Notify(term, os.Interrupt)

	go func() {
		<-term
		close(stop)
		conn.Close()
	}()

	var index uint64
	var currSendTime, currRecvTime int64
	remoteAddr := &net.UDPAddr{
		IP:   c.DestIp,
		Port: c.DestPort,
	}

	sendMsg := func() error {
		msg := fmt.Sprintf("%s%d", MagicHeader, index)
		_, err := conn.WriteToUDP([]byte(msg), remoteAddr)
		if err != nil {
			PrintT("Send msg [%d] to addr: %s error: %v\n", index, remoteAddr.String(), err)
			return err
		}

		currSendTime = time.Now().UnixNano()
		PrintT("Send msg [%d] to addr: %s\n", index, remoteAddr.String())
		index++
		return nil
	}

	recvEcho := func() error {
		var buf [256]byte
		n, remoteAddr, err := conn.ReadFromUDP(buf[:])
		if err != nil {
			PrintT("Read from UDP error: %v\n", err)
			return nil
		}
		if n < HeaderLen || string(buf[:HeaderLen]) != MagicHeader {
			PrintT("UDP message format error\n")
			return nil
		}

		var durationMs float64
		var durationStr string
		currRecvTime = time.Now().UnixNano()
		msg := string(buf[HeaderLen:n])

		durationMs = float64(currRecvTime-currSendTime) / float64(time.Millisecond)
		if durationMs > 0.0 {
			durationStr = fmt.Sprintf("%.3fms", durationMs)
		} else {
			durationStr = "-"
		}
		PrintT("Received echo [%s] from addr: %s, interval: %s\n", msg, remoteAddr.String(), durationStr)

		return nil
	}

	for {
		startTime := time.Now()
		err = conn.SetDeadline(startTime.Add(2500 * time.Millisecond))
		if err != nil {
			PrintT("Set conn error: %v\n", err)
			goto end
		}
		err = sendMsg()
		if err == nil {
			recvEcho()
		}

	end:
		endTime := time.Now()
		select {
		case <-time.After(3*time.Second - endTime.Sub(startTime)):
		case <-stop:
			PrintT("Program exited.\n")
			return
		}
	}
}
