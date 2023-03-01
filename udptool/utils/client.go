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
	DestIp       net.IP
	DestPort     int
	DataSize     int
	SendInterval int
}

func (c *Client) Run() {
	// connect using ephemeral local address and port
	remoteAddr := &net.UDPAddr{
		IP:   c.DestIp,
		Port: c.DestPort,
	}
	conn, err := net.DialUDP("udp4", nil, remoteAddr)
	if err != nil {
		PrintTee(nil, "dial error %v\n", err)
		return
	}

	// retrieve local port
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
	PrintTee(logger, "Connection setup at addr: %s, port %d\n", uaddr.IP.String(), uaddr.Port)

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

	sendMsg := func() error {
		packet := MakePacket(index, uint16(c.DataSize))
		_, err := conn.Write(packet)
		if err != nil {
			PrintTee(logger, "Send msg [%d] to addr: %s error: %v\n", index, remoteAddr.String(), err)
			return err
		}

		currSendTime = time.Now().UnixNano()
		PrintTee(logger, "Send msg [%d] len [%d] to addr: %s\n", index, len(packet), remoteAddr.String())
		index++
		return nil
	}

	recvEcho := func() {
		buf := make([]byte, HeaderLen+c.DataSize)
		n, err := conn.Read(buf[:])
		if err != nil {
			PrintTee(logger, "Read from UDP error: %v\n", err)
			return
		}

		if n < HeaderLen {
			PrintTee(logger, "UDP message format error\n")
			return
		}
		packet := buf[:n]
		header, err := CheckPacket(packet)
		if err != nil {
			PrintTee(logger, "UDP data packet error: %v\n", err)
			return
		}

		var durationMs float64
		var durationStr string
		currRecvTime = time.Now().UnixNano()
		durationMs = float64(currRecvTime-currSendTime) / float64(time.Millisecond)
		if durationMs >= 0.0 {
			durationStr = fmt.Sprintf("%.3fms", durationMs)
		} else {
			durationStr = "-"
		}
		PrintTee(logger, "Received echo [%d] len [%d] from addr: %s, interval: %s\n", header.Index, n, remoteAddr.String(), durationStr)
	}

	for {
		startTime := time.Now()
		err = conn.SetDeadline(startTime.Add(time.Duration(c.SendInterval)*time.Second - 100*time.Millisecond))
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
		case <-time.After(time.Duration(c.SendInterval)*time.Second - endTime.Sub(startTime)):
		case <-stop:
			PrintTee(logger, "Program exited.\n")
			return
		}
	}
}
