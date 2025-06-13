//go:build linux
// +build linux

package main

import (
	"fmt"
	l "kernelKoala/internal/logger"
	header "kernelKoala/internal/tui"
	network "kernelKoala/pkg/networkTraffic"
	"os"
)

func main() {
	header.PrintHeader()
	config := l.DefaultConfig()
	log, err := l.NewLogger(config)
	if err != nil {
		fmt.Println("log not configured")
		os.Exit(1)
	}
	network.NetworkTrafficCapture(log)
}
