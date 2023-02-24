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
	msgQueueSize  = 64
	maxConnection = 8
)

type Server struct {
	BindIp   net.IP
	BindPort int
}

type Peer struct {
	Msg              chan string
	LastReceivedTime int64
}

func (s *Server) Run() {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{
		IP:   s.BindIp,
		Port: s.BindPort,
	})
	if err != nil {
		PrintT("listen error %v\n", err)
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
		PrintT("resolve UDPAddr error %v\n", err)
		return
	}

	PrintT("Listen successful addr: %s, port %d\n", uaddr.String(), uaddr.Port)

	stop := make(chan struct{})
	term := make(chan os.Signal, 1)
	signal.Notify(term, syscall.SIGTERM)
	signal.Notify(term, os.Interrupt)

	go func() {
		<-term
		close(stop)
		conn.Close()
	}()

	echoReply := func(remote *net.UDPAddr, msg string) {
		err := conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
		if err != nil {
			PrintT("Send error: %v\n", err)
			return
		}

		sendMsg := fmt.Sprintf("%s%s", MagicHeader, msg)
		_, err = conn.WriteToUDP([]byte(sendMsg), remote)
		if err != nil {
			PrintT("Send echo [%s] to addr: %s error: %v\n", msg, remote.String(), err)
			return
		}

		PrintT("Send echo [%s] to addr: %s\n", msg, remote.String())
	}

	pruneConnection := func(now int64) bool {
		if len(connMap) < maxConnection {
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
			PrintT("Reached maximum connection. Remove outdated connection from addr: %s\n", delAddr)
			return true
		}

		return false
	}

	var wg sync.WaitGroup
	for {
		select {
		case <-stop:
			for addr, peer := range connMap {
				delete(connMap, addr)
				peer.Msg <- "quit"
			}
			wg.Wait()
			PrintT("Program exited\n")
			return
		default:
		}

		var buf [256]byte
		/*
			err = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			if err != nil {
				PrintT("Read error: %v\n", err)
				continue
			}
		*/

		n, remoteAddr, err := conn.ReadFromUDP(buf[:])
		if err != nil {
			PrintT("Read from UDP error: %v\n", err)
			continue
		}
		if n < HeaderLen || string(buf[:HeaderLen]) != MagicHeader {
			PrintT("UDP message format error\n")
			continue
		}

		recvTime := time.Now().UnixNano()
		msg := string(buf[HeaderLen:n])
		peer, ok := connMap[remoteAddr.String()]
		if ok {
			// existing connection
			peer.LastReceivedTime = recvTime
			peer.Msg <- msg
		} else {
			// new connection
			if !pruneConnection(recvTime) {
				PrintT("Reached maximum connection. Discard new msg [%s] from addr: %s\n", msg, remoteAddr.String())
				continue
			}

			// setup new routine for connection
			peer = &Peer{
				Msg:              make(chan string, msgQueueSize),
				LastReceivedTime: recvTime,
			}
			connMap[remoteAddr.String()] = peer
			PrintT("Setup new connection from addr: %s\n", remoteAddr.String())

			wg.Add(1)
			go func(remote *net.UDPAddr, peer *Peer) {
				defer wg.Done()
				defer close(peer.Msg)

				for {
					select {
					case <-time.After(1 * time.Hour):
						PrintT("Idle timeout. Stop echo routine for addr: %s\n", remote.String())
						return
					case msg := <-peer.Msg:
						PrintT("Received msg [%s] from addr: %s\n", msg, remote.String())
						if msg == "quit" {
							PrintT("Stop echo routine for addr: %s\n", remote.String())
							return
						}
						echoReply(remote, msg)
					}
				}
			}(remoteAddr, peer)

			peer.Msg <- msg
		}

	}
}
