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
		PrintTee(nil, "listen error %v\n", err)
		return
	}

	// retrieve port
	laddr := conn.LocalAddr()
	uaddr, err := net.ResolveUDPAddr(
		laddr.Network(),
		laddr.String(),
	)
	if err != nil {
		PrintTee(nil, "resolve UDPAddr error %v\n", err)
		return
	}

	// init log
	logger := &LogWriter{
		Name: fmt.Sprintf("client-%s_%d", uaddr.IP.String(), uaddr.Port),
	}
	defer logger.Close()
	PrintTee(logger, "Connection setup, addr: %s, port %d\n", uaddr.IP.String(), uaddr.Port)

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
		indexStr := fmt.Sprintf("%d", index)
		cs := StringChecksum(indexStr)
		msg := fmt.Sprintf("%s%s", cs, indexStr)
		_, err := conn.WriteToUDP([]byte(msg), remoteAddr)
		if err != nil {
			PrintTee(logger, "Send msg [%d] to addr: %s error: %v\n", index, remoteAddr.String(), err)
			return err
		}

		currSendTime = time.Now().UnixNano()
		PrintTee(logger, "Send msg [%d] to addr: %s\n", index, remoteAddr.String())
		index++
		return nil
	}

	recvEcho := func() error {
		var buf [256]byte
		n, remoteAddr, err := conn.ReadFromUDP(buf[:])
		if err != nil {
			PrintTee(logger, "Read from UDP error: %v\n", err)
			return nil
		}

		if n < HeaderLen {
			PrintTee(logger, "UDP message format error\n")
			return nil
		}

		msg := string(buf[HeaderLen:n])
		cs := StringChecksum(msg)
		if string(buf[:HeaderLen]) != cs {
			PrintTee(logger, "UDP message checksum error\n")
			return nil
		}

		var durationMs float64
		var durationStr string
		currRecvTime = time.Now().UnixNano()
		durationMs = float64(currRecvTime-currSendTime) / float64(time.Millisecond)
		if durationMs > 0.0 {
			durationStr = fmt.Sprintf("%.3fms", durationMs)
		} else {
			durationStr = "-"
		}
		PrintTee(logger, "Received echo [%s] from addr: %s, interval: %s\n", msg, remoteAddr.String(), durationStr)

		return nil
	}

	for {
		startTime := time.Now()
		err = conn.SetDeadline(startTime.Add(2500 * time.Millisecond))
		if err != nil {
			PrintTee(logger, "Set conn error: %v\n", err)
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
			PrintTee(logger, "Program exited.\n")
			return
		}
	}
}
