package network

import (
	"fmt"
	"sync"
	"text/tabwriter"
)

type ifaceTablePrinter struct {
	chTCP     chan PayLoadTc
	chUDP     chan PayLoadTc
	chICMP    chan PayLoadTc
	chUnknown chan PayLoadTc

	tableWriters map[string]*tabwriter.Writer
	tableLocks   map[string]*sync.Mutex
	writerLock   sync.Mutex
	tableData    map[string][]Event // Store limited rows per interface

}

// Initialize table printer with channel forwarding
func (i *ifaceTablePrinter) InitTable() {
	i.tableWriters = make(map[string]*tabwriter.Writer)
	i.tableLocks = make(map[string]*sync.Mutex)

	go i.forward(i.chTCP)
	go i.forward(i.chUDP)
	go i.forward(i.chICMP)
	go i.forward(i.chUnknown)
}

// Forward events from channel to their interface table
func (i *ifaceTablePrinter) forward(ch <-chan PayLoadTc) {
	for ev := range ch {
		iface := ev.Iface
		i.printForInterface(iface, ev)
	}
}

func (i *ifaceTablePrinter) printForInterface(iface string, ev PayLoadTc) {
	i.writerLock.Lock()
	if _, exists := i.tableData[iface]; !exists {
		i.tableData[iface] = make([]Event, 0)
		i.tableLocks[iface] = &sync.Mutex{}
	}
	i.tableData[iface] = append(i.tableData[iface], ev.Event)
	if len(i.tableData[iface]) > 10 { // limit to 10 rows (~25% height)
		i.tableData[iface] = i.tableData[iface][len(i.tableData[iface])-10:]
	}
	lk := i.tableLocks[iface]
	i.writerLock.Unlock()

	lk.Lock()
	defer lk.Unlock()

	// Clear terminal and move cursor to top
	fmt.Print("\033[2J\033[H")

	fmt.Printf("╔══════════════════════════════════════════════════════╗\n")
	fmt.Printf("║ INTERFACE: %-43s ║\n", iface)
	fmt.Printf("╠═════════╦══════════╦══════════════╦══════════╦══════════════╦══════════╦════════════╣\n")
	fmt.Printf("║ Protocol║ Direction║ Source       ║ Src Port ║ Destination  ║ Dst Port ║ Flags      ║\n")
	fmt.Printf("╠═════════╬══════════╬══════════════╬══════════╬══════════════╬══════════╬════════════╣\n")

	for _, e := range i.tableData[iface] {
		proto := protocolName(e.Protocol)
		dir := "Ingress"
		if e.Direction == 1 {
			dir = "Egress"
		}
		src := intToIP(e.SrcIP)
		dst := intToIP(e.DstIP)
		flags := tcpFlagsToString(e.TcpFlags)
		fmt.Printf("║ %-8s║ %-9s║ %-12s ║ %-8d║ %-12s ║ %-8d║ %-10s ║\n",
			proto, dir, src, e.SrcPort, dst, e.DstPort, flags)
	}

	fmt.Printf("╚═════════╩══════════╩══════════════╩══════════╩══════════════╩══════════╩════════════╝\n")
}

// Convert protocol number to name
func protocolName(proto uint8) string {
	switch proto {
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	case 1:
		return "ICMP"
	default:
		return fmt.Sprintf("PROTO(%d)", proto)
	}
}
