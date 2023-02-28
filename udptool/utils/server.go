package utils

import (
	"fmt"
	"math"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	MsgQueueSize  = 64
	MaxConnection = 1024
)

type Server struct {
	BindIp   net.IP
	BindPort int
}

type Peer struct {
	Remote           *net.UDPAddr
	Logger           *LogWriter
	Msg              chan string
	LastReceivedTime int64
}

func (s *Server) Run() {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{
		IP:   s.BindIp,
		Port: s.BindPort,
	})
	if err != nil {
		PrintTee(nil, "listen error %v\n", err)
		return
	}

	// map only used with in the main thread, so don't need sync
	connMap := make(map[string]*Peer)

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
		Name: fmt.Sprintf("server-%s_%d", uaddr.IP.String(), uaddr.Port),
	}
	defer logger.Close()
	PrintTee(logger, "Listen successful, addr: %s, port %d\n", uaddr.IP.String(), uaddr.Port)
	os.MkdirAll("peer/", os.ModePerm)

	stop := make(chan struct{})
	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM)
	signal.Notify(term, os.Interrupt)

	go func() {
		<-term
		close(stop)
		conn.Close()
	}()

	echoReply := func(peer *Peer, msg string) {
		err := conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
		if err != nil {
			PrintTee(peer.Logger, "Send error: %v\n", err)
			return
		}

		cs := StringChecksum(msg)
		sendMsg := fmt.Sprintf("%s%s", cs, msg)
		_, err = conn.WriteToUDP([]byte(sendMsg), peer.Remote)
		if err != nil {
			PrintTee(peer.Logger, "Send echo [%s] to addr: %s error: %v\n", msg, peer.Remote.String(), err)
			return
		}

		PrintTee(peer.Logger, "Send echo [%s] to addr: %s\n", msg, peer.Remote.String())
	}

	pruneConnection := func(now int64) bool {
		if len(connMap) < MaxConnection {
			return true
		}
		// clean up outdated peer if max connection is reached
		var oldest int64 = math.MaxInt64
		var delAddr string
		var delPeer *Peer
		for addr, peer := range connMap {
			if peer.LastReceivedTime < oldest {
				oldest = peer.LastReceivedTime
				delAddr = addr
				delPeer = peer
			}
		}

		if now-oldest > int64(300*time.Second) {
			delete(connMap, delAddr)
			delPeer.Msg <- "quit"
			PrintTee(logger, "Reached maximum connection. Remove outdated connection from addr: %s\n", delAddr)
			return true
		}

		return false
	}

	// transmission start
	var wg sync.WaitGroup
	for {
		select {
		case <-stop:
			for addr, peer := range connMap {
				delete(connMap, addr)
				peer.Msg <- "quit"
			}
			wg.Wait()
			PrintTee(logger, "Program exited\n")
			return
		default:
		}

		var buf [256]byte
		/*
			err = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			if err != nil {
				PrintTee("Read error: %v\n", err)
				continue
			}
		*/

		n, remoteAddr, err := conn.ReadFromUDP(buf[:])
		if err != nil {
			PrintTee(logger, "Read from UDP error: %v\n", err)
			continue
		}
		if n < HeaderLen {
			PrintTee(logger, "UDP message format error\n")
			continue
		}

		msg := string(buf[HeaderLen:n])
		cs := StringChecksum(msg)
		if string(buf[:HeaderLen]) != cs {
			PrintTee(logger, "UDP message checksum error\n")
			continue
		}

		recvTime := time.Now().UnixNano()
		peer, ok := connMap[remoteAddr.String()]
		if ok {
			// existing connection
			peer.LastReceivedTime = recvTime
			peer.Msg <- msg
		} else {
			// new connection
			if !pruneConnection(recvTime) {
				PrintTee(logger, "Reached maximum connection. Discard new msg [%s] from addr: %s\n", msg, remoteAddr.String())
				continue
			}

			// setup new routine for connection
			peer = &Peer{
				Remote: remoteAddr,
				Logger: &LogWriter{
					Prefix: "peer/",
					Name:   fmt.Sprintf("peer-%s_%d", remoteAddr.IP.String(), remoteAddr.Port),
				},
				Msg:              make(chan string, MsgQueueSize),
				LastReceivedTime: recvTime,
			}
			connMap[remoteAddr.String()] = peer
			PrintTee(logger, "Setup new connection from addr: %s\n", remoteAddr.String())

			wg.Add(1)
			go func(peer *Peer) {
				defer wg.Done()
				defer peer.Logger.Close()
				defer close(peer.Msg)

				for {
					select {
					case <-time.After(1 * time.Hour):
						PrintTee(peer.Logger, "Idle timeout. Stop echo routine for addr: %s\n", peer.Remote.String())
						return
					case msg := <-peer.Msg:
						PrintTee(peer.Logger, "Received msg [%s] from addr: %s\n", msg, peer.Remote.String())
						if msg == "quit" {
							PrintTee(peer.Logger, "Stop echo routine for addr: %s\n", peer.Remote.String())
							return
						}
						echoReply(peer, msg)
					}
				}
			}(peer)

			peer.Msg <- msg
		}

	}
}
