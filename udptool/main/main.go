package main

import (
	"flag"
	"fmt"
	"net"
	"strconv"
	"strings"
	"udptool/utils"
)

type Mode int

const (
	ClientMode Mode = iota
	ServerMode
)

func main() {
	// cmd line args
	var input, inputP, inputS, inputC string
	var mode Mode
	var ip net.IP
	var port int
	var dataSize int
	var sendInterval int
	var err error

	flag.Usage = func() {
		fmt.Printf("\n")
		fmt.Printf("Usage of UDP tool:\n")
		fmt.Printf("\t-p server mode:   Specify port_number for listening.\n")
		fmt.Printf("\t-s server mode:   Specify ip_address:port_number for listening.\n")
		fmt.Printf("\t-c client mode:   Specify ip_address:port_number for connecting.\n")
		fmt.Printf("\t-n client mode:   Specify data size to send in one packet.\n")
		fmt.Printf("\t-t client mode:   Specify send interval.\n")
		fmt.Printf("\t(ip_address must be legal IPv4 address.)\n")
		fmt.Printf("\t(port_number must have the range between 1 and 65535.)\n")
		fmt.Printf("\n")
	}

	flag.StringVar(&inputP, "p", "", "Hosting port for server mode")
	flag.StringVar(&inputS, "s", "", "Hosting address for server mode")
	flag.StringVar(&inputC, "c", "", "Connecting address for client mode")
	flag.IntVar(&dataSize, "n", 0, "Additional data packet size for client mode")
	flag.IntVar(&sendInterval, "t", 3, "Send interval for client mode")
	flag.Parse()

	if len(inputP) > 0 {
		port, err = strconv.Atoi(inputP)
		if err != nil || port <= 0 || port > 0xffff {
			fmt.Printf("Error: port number is incorrect.\n")
			flag.Usage()
			return
		} else {
			mode = ServerMode
			ip = net.IPv4zero
		}
	} else {
		if len(inputC) > 0 {
			mode = ClientMode
			input = inputC
			if dataSize < 0 {
				dataSize = 0
			} else if dataSize > utils.MaxPayloadSize {
				dataSize = utils.MaxPayloadSize
			}
			if sendInterval < 1 {
				sendInterval = 1
			}
		} else {
			mode = ServerMode
			input = inputS
		}

		if len(input) == 0 {
			fmt.Printf("Error: missing arguments.\n")
			flag.Usage()
			return
		}

		strArr := strings.Split(input, ":")
		if len(strArr) != 2 {
			fmt.Printf("Error: address format error.\n")
			flag.Usage()
			return
		}

		ip = net.ParseIP(strArr[0])
		if ip == nil {
			fmt.Printf("Error: ip address is incorrect!\n")
			flag.Usage()
			return
		}

		port, err = strconv.Atoi(strArr[1])
		if err != nil || port <= 0 || port > 0xffff {
			fmt.Printf("Error: port number is incorrect.\n")
			flag.Usage()
			return
		}
	}

	switch mode {
	case ServerMode:
		server := &utils.Server{
			BindIp:   ip,
			BindPort: port,
		}
		server.Run()

	case ClientMode:
		client := &utils.Client{
			DestIp:       ip,
			DestPort:     port,
			DataSize:     dataSize,
			SendInterval: sendInterval,
		}
		client.Run()
	}
}
