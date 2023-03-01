package utils

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	MsgQueueSize       = 64
	MaxConnection      = 1024
	RateReportInterval = 5
	PeerTimeoutMinute  = 10
)

type Server struct {
	BindIp   net.IP
	BindPort int
}

type Message struct {
	Header *PacketHeader
	Buffer *[MaxBufferSize]byte
	Packet []byte
}

type Peer struct {
	Remote           *net.UDPAddr
	Logger           *LogWriter
	Msg              chan *Message
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

	connMap := make(map[string]*Peer)
	var connMapMutex sync.Mutex

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

	var packetBuffers PacketBufferPool
	packetBuffers.Init(MaxConnection + MaxConnection/2) // 150% max connection

	go func() {
		<-term
		close(stop)
		conn.Close()
	}()

	// rate report routine
	var sendByte, recvByte uint64
	var sendRate, recvRate float64
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(RateReportInterval * time.Second):
				sendRate = float64(atomic.LoadUint64(&sendByte)) / (RateReportInterval * 1000) // kB/s
				recvRate = float64(atomic.LoadUint64(&recvByte)) / (RateReportInterval * 1000) // kB/s
				atomic.StoreUint64(&sendByte, 0)
				atomic.StoreUint64(&recvByte, 0)
				PrintTee(logger, "Average send %.2f kB/s, recv %.2f kB/s\n", sendRate, recvRate)
			}
		}
	}()

	sendReply := func(peer *Peer, msg *Message) {
		err := conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
		if err != nil {
			PrintTee(peer.Logger, "Send error: %v\n", err)
			return
		}

		n, err := conn.WriteToUDP(msg.Packet, peer.Remote)
		if err != nil {
			PrintTee(peer.Logger, "Send echo [%d] to addr: %s error: %v\n", msg.Header.Index, peer.Remote.String(), err)
			return
		}

		atomic.AddUint64(&sendByte, uint64(n))
		PrintTee(peer.Logger, "Send echo [%d] len [%d] to addr: %s\n", msg.Header.Index, n, peer.Remote.String())
	}

	// transmission start
	var wg sync.WaitGroup

	for {
		select {
		case <-stop:
			connMapMutex.Lock()
			for addr, peer := range connMap {
				delete(connMap, addr)
				peer.Msg <- nil
			}
			connMapMutex.Unlock()
			wg.Wait()
			PrintTee(logger, "Program exited\n")
			return
		default:
		}

		// allocate a new packet buffer for every read
		buf := packetBuffers.Get()
		/*
			err = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			if err != nil {
				PrintTee("Read error: %v\n", err)
				continue
			}
		*/

		n, remoteAddr, err := conn.ReadFromUDP(buf[:])
		if err != nil {
			packetBuffers.Put(buf)
			PrintTee(logger, "Read from UDP error: %v\n", err)
			continue
		}
		atomic.AddUint64(&recvByte, uint64(n))

		if n < HeaderLen {
			packetBuffers.Put(buf)
			PrintTee(logger, "UDP message format error\n")
			continue
		}

		packet := buf[:n]
		header, err := CheckPacket(packet)
		if err != nil {
			packetBuffers.Put(buf)
			PrintTee(logger, "UDP data packet error: %v\n", err)
			continue
		}

		recvTime := time.Now().UnixNano()
		msg := &Message{
			Header: header,
			Buffer: buf,
			Packet: packet,
		}

		connMapMutex.Lock()
		peer, ok := connMap[remoteAddr.String()]
		connMapMutex.Unlock()

		if ok {
			// existing connection
			peer.LastReceivedTime = recvTime
			peer.Msg <- msg
		} else {
			// create new connection if there is room
			connMapMutex.Lock()
			if len(connMap) >= MaxConnection {
				connMapMutex.Unlock()
				packetBuffers.Put(buf)
				PrintTee(logger, "Reached maximum connection. Discard new msg [%d] from addr: %s\n", msg.Header.Index, remoteAddr.String())
				continue
			}
			connMapMutex.Unlock()

			// setup new routine for connection
			peer = &Peer{
				Remote: remoteAddr,
				Logger: &LogWriter{
					Prefix: "peer/",
					Name:   fmt.Sprintf("peer-%s_%d", remoteAddr.IP.String(), remoteAddr.Port),
				},
				Msg:              make(chan *Message, MsgQueueSize),
				LastReceivedTime: recvTime,
			}
			connMapMutex.Lock()
			connMap[remoteAddr.String()] = peer
			connMapMutex.Unlock()
			peer.Msg <- msg

			PrintTee(logger, "Setup new connection from addr: %s\n", remoteAddr.String())

			// peer echo routine
			wg.Add(1)
			go func(peer *Peer) {
				defer wg.Done()
				defer peer.Logger.Close()
				defer close(peer.Msg)

				PrintTee(peer.Logger, "New peer created for connection from addr: %s\n", peer.Remote.String())

				for {
					select {
					case <-time.After(PeerTimeoutMinute * time.Minute):
						// actively self cleaning
						connMapMutex.Lock()
						delete(connMap, peer.Remote.String())
						connMapMutex.Unlock()

						PrintTee(peer.Logger, "Idle timeout. Stop peer routine for addr: %s\n", peer.Remote.String())
						PrintTee(logger, "Idle timeout. Connection removed from addr: %s\n", peer.Remote.String())
						return
					case msg := <-peer.Msg:
						if msg == nil {
							PrintTee(peer.Logger, "Stop peer routine for addr: %s\n", peer.Remote.String())
							PrintTee(logger, "Connection removed from addr: %s\n", peer.Remote.String())
							return
						}
						PrintTee(peer.Logger, "Received msg [%d] len [%d] from addr: %s\n", msg.Header.Index, len(msg.Packet), peer.Remote.String())
						sendReply(peer, msg)

						// Terminate further usage of msg object. This makes the garbage collector's life easier
						msg.Header = nil
						msg.Packet = nil
						packetBuffers.Put(msg.Buffer)
					}
				}
			}(peer)
		}
	}
}
