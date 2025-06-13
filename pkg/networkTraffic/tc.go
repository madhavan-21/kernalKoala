package network

import (
	"bytes"
	"encoding/binary"
	"fmt"
	l "kernelKoala/internal/logger"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/perf"
	"github.com/vishvananda/netlink"
)

type Event struct {
	SrcIP     uint32
	DstIP     uint32
	SrcPort   uint16
	DstPort   uint16
	Protocol  uint8
	Direction uint8
	TcpFlags  uint8
}

func NetworkTrafficCapture(log *l.Logger) {
	if len(os.Args) < 2 {
		log.Fatal("please specify the network interface")
	}

	ifaceName := os.Args[1]
	log.Info("getted iface name : %s", ifaceName)

	arch := runtime.GOARCH
	var archDir string

	switch arch {
	case "amd64":
		archDir = "x86_64"
	case "arm64":
		archDir = "aarch64"
	case "riscv64":
		archDir = "riscv64"
	default:
		log.Fatal("Unsupported architecture: %s", arch)
	}

	bpfPath := fmt.Sprintf("../bpf/network/build/%s/tc.o", archDir)
	spec, err := ebpf.LoadCollectionSpec(bpfPath)
	if err != nil {
		log.Fatal("failed to load eBPF spec from %s: %v", bpfPath, err)
	}

	log.Info("Successfully loaded BPF spec for : %s", archDir)

	var objs struct {
		TcIngress *ebpf.Program `ebpf:"tc_ingress"`
		TcEgress  *ebpf.Program `ebpf:"tc_egress"`
		Events    *ebpf.Map     `ebpf:"events"`
	}

	if err := spec.LoadAndAssign(&objs, nil); err != nil {
		log.Fatal("failed to load eBPF spec %v", err)
	}

	defer objs.TcIngress.Close()
	defer objs.TcEgress.Close()
	defer objs.Events.Close()

	link, err := netlink.LinkByName(ifaceName)
	if err != nil {
		log.Fatal("error on finding link by name : %v", err.Error())
	}
	log.Info("getted netlink name : %v", link)
	qdiscs, err := netlink.QdiscList(link)
	if err != nil {
		log.Fatal("listening qdsics: %v", err)
	}

	clsactExists := false

	for _, qdisc := range qdiscs {
		if _, ok := qdisc.(*netlink.GenericQdisc); ok && qdisc.Type() == "clsact" {
			clsactExists = true
			break
		}
	}

	if clsactExists == false {
		qdisc := &netlink.GenericQdisc{
			QdiscAttrs: netlink.QdiscAttrs{
				LinkIndex: link.Attrs().Index,
				Handle:    netlink.MakeHandle(0xffff, 0),
				Parent:    netlink.HANDLE_CLSACT,
			},
			QdiscType: "clsact",
		}
		if err := netlink.QdiscAdd(qdisc); err != nil {
			log.Fatal("adding clsact qdisc: %v", err)
		}
		log.Info("Added clsact qdisc")
	} else {
		log.Info("clsact qdisc already exists")
	}

	ingressFilter := &netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: link.Attrs().Index,
			Parent:    netlink.HANDLE_MIN_INGRESS,
			Handle:    netlink.MakeHandle(0, 1),
			Protocol:  syscall.ETH_P_ALL,
		},
		Fd:           objs.TcIngress.FD(),
		Name:         "tc_ingress",
		DirectAction: true,
	}

	if err := netlink.FilterAdd(ingressFilter); err != nil {
		log.Fatal("adding ingress filter: %v", err)
	}
	fmt.Println("Added ingress filter")

	egressFilter := &netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: link.Attrs().Index,
			Parent:    netlink.HANDLE_MIN_EGRESS,
			Handle:    netlink.MakeHandle(0, 1),
			Protocol:  syscall.ETH_P_ALL,
		},
		Fd:           objs.TcEgress.FD(),
		Name:         "tc_egress",
		DirectAction: true,
	}
	if err := netlink.FilterAdd(egressFilter); err != nil {
		log.Fatal("adding egress filter: %v", err)
	}
	fmt.Println("Added egress filter")

	// perf reader
	reader, err := perf.NewReader(objs.Events, os.Getpagesize())
	if err != nil {
		log.Fatal("creating perf reader: %v", err)
	}
	defer reader.Close()

	// read events
	go func() {
		for {
			record, err := reader.Read()
			if err != nil {
				if err == perf.ErrClosed {
					return
				}
				log.Info("reading from perf event reader: %s", err)
				continue
			}

			if record.LostSamples != 0 {
				log.Info(
					"perf event ring buffer full, dropped %d samples",
					record.LostSamples,
				)
				continue
			}

			var event Event
			if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
				log.Info("parsing perf event: %s", err)
				continue
			}
			printPacket(event)

		}
	}()

	fmt.Printf("eBPF programs attached to interface %s\n", ifaceName)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	fmt.Println("Received interrupt, cleaning up...")

	// Clean up filters
	filters, err := netlink.FilterList(link, netlink.HANDLE_MIN_INGRESS)
	if err != nil {
		log.Info("error listing ingress filters: %v", err)
	} else {
		for _, filter := range filters {
			if bpfFilter, ok := filter.(*netlink.BpfFilter); ok && bpfFilter.Name == "tc_ingress" {
				if err := netlink.FilterDel(bpfFilter); err != nil {
					log.Info("error removing ingress filter: %v", err)
				} else {
					fmt.Println("Removed ingress filter")
				}
			}
		}
	}

	filterss, err := netlink.FilterList(link, netlink.HANDLE_MIN_EGRESS)
	if err != nil {
		log.Info("error listing egress filters: %v", err)
	} else {
		for _, filter := range filterss {
			if bpfFilter, ok := filter.(*netlink.BpfFilter); ok && bpfFilter.Name == "tc_egress" {
				if err := netlink.FilterDel(bpfFilter); err != nil {
					log.Info("error removing egress filter: %v", err)
				} else {
					fmt.Println("Removed egress filter")
				}
			}
		}
	}

	qdisc := &netlink.GenericQdisc{
		QdiscAttrs: netlink.QdiscAttrs{
			LinkIndex: link.Attrs().Index,
			Handle:    netlink.MakeHandle(0xffff, 0),
			Parent:    netlink.HANDLE_CLSACT,
		},
		QdiscType: "clsact",
	}
	if err := netlink.QdiscDel(qdisc); err != nil {
		log.Fatal("deleting clsact qdisc: %v", err)
	}
	fmt.Println("Deleted clsact qdisc")

}

func loadBpfSpec(path string) (*ebpf.CollectionSpec, error) {
	spec, err := ebpf.LoadCollectionSpec(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load BPF spec : %v", err)
	}

	if eventMap, ok := spec.Maps["events"]; ok == true {
		eventMap.Type = ebpf.PerfEventArray
	}
	return spec, nil
}

func printPacket(event Event) {
	direction := "Ingress"
	if event.Direction == 1 {
		direction = "Egress"
	}

	srcIP := intToIP(event.SrcIP)
	dstIP := intToIP(event.DstIP)

	srcDomain := resolveIP(srcIP)
	dstDomain := resolveIP(dstIP)

	switch event.Protocol {
	case 6: // TCP
		flags := tcpFlagsToString(event.TcpFlags)
		fmt.Printf("%s TCP: src=%s (%s):%d -> dst=%s (%s):%d | proto=%d | flags=%s\n",
			direction,
			srcIP, srcDomain, event.SrcPort,
			dstIP, dstDomain, event.DstPort,
			event.Protocol,
			flags,
		)
	case 17: // UDP
		fmt.Printf("%s UDP: src=%s (%s):%d -> dst=%s (%s):%d | proto=%d\n",
			direction,
			srcIP, srcDomain, event.SrcPort,
			dstIP, dstDomain, event.DstPort,
			event.Protocol,
		)
	case 1: // ICMP
		fmt.Printf("%s ICMP: src=%s (%s) -> dst=%s (%s) | proto=%d\n",
			direction,
			srcIP, srcDomain,
			dstIP, dstDomain,
			event.Protocol,
		)
	default:
		fmt.Printf("%s UNKNOWN: src=%s (%s) -> dst=%s (%s) | proto=%d\n",
			direction,
			srcIP, srcDomain,
			dstIP, dstDomain,
			event.Protocol,
		)
	}
}

// func printPacket(event Event) {
// 	direction := "Ingress"
// 	if event.Direction == 1 {
// 		direction = "Egress"
// 	}

// 	switch event.Protocol {
// 	case 6: // TCP
// 		flags := tcpFlagsToString(event.TcpFlags)
// 		fmt.Printf("%s TCP: src=%s:%d -> dst=%s:%d | proto=%d | flags=%s\n",
// 			direction,
// 			intToIP(event.SrcIP), event.SrcPort,
// 			intToIP(event.DstIP), event.DstPort,
// 			event.Protocol,
// 			flags,
// 		)
// 	case 17: // UDP
// 		fmt.Printf("%s UDP: src=%s:%d -> dst=%s:%d | proto=%d\n",
// 			direction,
// 			intToIP(event.SrcIP), event.SrcPort,
// 			intToIP(event.DstIP), event.DstPort,
// 			event.Protocol,
// 		)
// 	case 1: // ICMP
// 		fmt.Printf("%s ICMP: src=%s -> dst=%s | proto=%d\n",
// 			direction,
// 			intToIP(event.SrcIP),
// 			intToIP(event.DstIP),
// 			event.Protocol)
// 	case 2: // IGMP
// 		fmt.Printf("%s IGMP: src=%s -> dst=%s | proto=%d\n",
// 			direction,
// 			intToIP(event.SrcIP),
// 			intToIP(event.DstIP),
// 			event.Protocol)
// 	case 50:
// 		fmt.Printf("%s ESP (IPsec): src=%s -> dst=%s | proto=%d\n",
// 			direction,
// 			intToIP(event.SrcIP),
// 			intToIP(event.DstIP),
// 			event.Protocol)
// 	default:
// 		fmt.Printf("%s UNKNOWN: src=%s:%d -> dst=%s:%d | proto=%d\n",
// 			direction,
// 			intToIP(event.SrcIP), event.SrcPort,
// 			intToIP(event.DstIP), event.DstPort,
// 			event.Protocol,
// 		)
// 	}
// }

func tcpFlagsToString(flags uint8) string {
	flagNames := []struct {
		mask uint8
		name string
	}{
		{0x01, "FIN"},
		{0x02, "SYN"},
		{0x04, "RST"},
		{0x08, "PSH"},
		{0x10, "ACK"},
		{0x20, "URG"},
		{0x40, "ECE"},
		{0x80, "CWR"},
	}

	var result []string
	for _, f := range flagNames {
		if flags&f.mask != 0 {
			result = append(result, f.name)
		}
	}
	if len(result) == 0 {
		return "NONE"
	}
	return fmt.Sprintf("0x%x (%s)", flags, fmt.Sprintf("%s", result))
}
func intToIP(ip uint32) net.IP {
	return net.IPv4(byte(ip), byte(ip>>8), byte(ip>>16), byte(ip>>24))
}

func resolveIP(ip net.IP) string {
	names, err := net.LookupAddr(ip.String())
	if err != nil || len(names) == 0 {
		return "-"
	}
	return names[0]
}
