package network

import (
	"bytes"
	"encoding/binary"
	"fmt"
	l "kernelKoala/internal/logger"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/perf"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
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

type PayLoadTc struct {
	Event Event
	Iface string
}

func NetworkTrafficCapture(log *l.Logger) {
	// Validate args
	if len(os.Args) < 2 {
		log.Fatal("please specify the network interface")
	}

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

	// Locate and load BPF
	_, filename, _, _ := runtime.Caller(0)
	sourceDir := filepath.Dir(filename)
	bpfPath := filepath.Join(sourceDir, "../../bpf/network/build/tc-"+archDir+".o")

	spec, err := ebpf.LoadCollectionSpec(bpfPath)
	if err != nil {
		log.Fatal("failed to load eBPF spec from %s: %v", bpfPath, err)
	}

	var objs struct {
		TcIngress *ebpf.Program `ebpf:"tc_ingress"`
		TcEgress  *ebpf.Program `ebpf:"tc_egress"`
		Events    *ebpf.Map     `ebpf:"events"`
	}

	raiseMemlockLimit()
	if err := spec.LoadAndAssign(&objs, nil); err != nil {
		log.Fatal("failed to load eBPF spec %v", err)
	}
	defer objs.TcIngress.Close()
	defer objs.TcEgress.Close()
	defer objs.Events.Close()

	Interfaces, err := interfaceCollector()
	if err != nil {
		log.Fatal("failed to get interface name")
	}

	eventChan := make(chan PayLoadTc, 1000)
	var wg sync.WaitGroup

	// Optional consumer
	go func() {
		for evt := range eventChan {
			printPacket(evt.Event, evt.Iface)
		}
	}()

	for _, iface := range Interfaces {
		wg.Add(1)

		go func(iface net.Interface) {
			defer wg.Done()

			link, err := netlink.LinkByName(iface.Name)
			if err != nil {
				log.Fatal("link not found: %v", err)
			}

			qdiscs, _ := netlink.QdiscList(link)
			clsactExists := false
			for _, q := range qdiscs {
				if _, ok := q.(*netlink.GenericQdisc); ok && q.Type() == "clsact" {
					clsactExists = true
					break
				}
			}

			if !clsactExists {
				qdisc := &netlink.GenericQdisc{
					QdiscAttrs: netlink.QdiscAttrs{
						LinkIndex: link.Attrs().Index,
						Handle:    netlink.MakeHandle(0xffff, 0),
						Parent:    netlink.HANDLE_CLSACT,
					},
					QdiscType: "clsact",
				}
				_ = netlink.QdiscAdd(qdisc)
			}

			// Attach filters
			_ = netlink.FilterAdd(&netlink.BpfFilter{
				FilterAttrs: netlink.FilterAttrs{
					LinkIndex: link.Attrs().Index,
					Parent:    netlink.HANDLE_MIN_INGRESS,
					Handle:    netlink.MakeHandle(0, 1),
					Protocol:  syscall.ETH_P_ALL,
				},
				Fd:           objs.TcIngress.FD(),
				Name:         "tc_ingress",
				DirectAction: true,
			})
			_ = netlink.FilterAdd(&netlink.BpfFilter{
				FilterAttrs: netlink.FilterAttrs{
					LinkIndex: link.Attrs().Index,
					Parent:    netlink.HANDLE_MIN_EGRESS,
					Handle:    netlink.MakeHandle(0, 1),
					Protocol:  syscall.ETH_P_ALL,
				},
				Fd:           objs.TcEgress.FD(),
				Name:         "tc_egress",
				DirectAction: true,
			})

			reader, err := perf.NewReader(objs.Events, os.Getpagesize())
			if err != nil {
				log.Fatal("perf reader error: %v", err)
			}
			defer reader.Close()

			// Read events
			go func() {
				for {
					record, err := reader.Read()
					if err != nil {
						if err == perf.ErrClosed {
							return
						}
						continue
					}
					if record.LostSamples != 0 {
						continue
					}

					var event Event
					if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
						continue
					}

					eventChan <- PayLoadTc{
						Iface: iface.Name,
						Event: event,
					}
				}
			}()

			// Wait for Ctrl+C
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
			<-sig

			// Cleanup
			_ = netlink.QdiscDel(&netlink.GenericQdisc{
				QdiscAttrs: netlink.QdiscAttrs{
					LinkIndex: link.Attrs().Index,
					Handle:    netlink.MakeHandle(0xffff, 0),
					Parent:    netlink.HANDLE_CLSACT,
				},
				QdiscType: "clsact",
			})
		}(iface)
	}

	// Wait for all routines to finish
	wg.Wait()
	close(eventChan)
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

func printPacket(event Event, iface string) {
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
		fmt.Printf("%s TCP: src=%s (%s):%d -> dst=%s (%s):%d | proto=%d | flags=%s \n",
			direction,
			srcIP, srcDomain, event.SrcPort,
			dstIP, dstDomain, event.DstPort,
			event.Protocol,
			flags,
		)
	case 17: // UDP
		flags := tcpFlagsToString(event.TcpFlags)
		fmt.Printf("%s UDP: src=%s (%s):%d -> dst=%s (%s):%d | proto=%d | flags= %s | iface = %s \n",
			direction,
			srcIP, srcDomain, event.SrcPort,
			dstIP, dstDomain, event.DstPort,
			event.Protocol,
			flags,
			iface,
		)
	case 1: // ICMP
		flags := tcpFlagsToString(event.TcpFlags)
		fmt.Printf("%s ICMP: src=%s (%s) -> dst=%s (%s) | proto=%d |flags = %s, iface = %s \n",
			direction,
			srcIP, srcDomain,
			dstIP, dstDomain,
			event.Protocol,
			flags,
			iface,
		)
	default:
		flags := tcpFlagsToString(event.TcpFlags)
		fmt.Printf("%s UNKNOWN: src=%s (%s) -> dst=%s (%s) | proto=%d | flags = %s , iface = %s\n",
			direction,
			srcIP, srcDomain,
			dstIP, dstDomain,
			event.Protocol,
			flags,
			iface,
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

//raiseMemlockLimit() is added to allow the app to lock more memory, which is needed for eBPF programs and maps. Without it, the app may fail with "permission denied" errors. It solves this by raising the memory lock limit to unlimited.

func raiseMemlockLimit() {
	rLimit := &unix.Rlimit{
		Cur: unix.RLIM_INFINITY,
		Max: unix.RLIM_INFINITY,
	}
	if err := unix.Setrlimit(unix.RLIMIT_MEMLOCK, rLimit); err != nil {
		log.Fatalf("❌ Failed to raise rlimit: %v", err)
	}
}
