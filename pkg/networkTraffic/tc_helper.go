package network

import (
	"net"
)

func interfaceCollector() ([]net.Interface, error) {
	Interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	return Interfaces, nil
}
