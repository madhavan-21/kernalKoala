package network

import (
	"bytes"
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	l "kernelKoala/internal/logger"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

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
	ctx, cancel := context.WithCancel(context.Background())

	// Parse flags
	iface := flag.String("iface", "", "Network interface to attach (can also set IFACE env variable)")
	loopbackFlag := flag.Bool("loopback", true, "Set to false to allow localhost (loopback) traffic; default is true (drop loopback)")
	flag.Parse()

	// Resolve interface: flag → env → default
	if *iface == "" {
		if envIface := os.Getenv("IFACE"); envIface != "" {
			*iface = envIface
		} else {
			*iface = "lo"
			l.Info("Interface not provided; defaulting to 'lo'")
		}
	}

	// Resolve loopback: flag → env → default
	loopback := *loopbackFlag
	if envVal, ok := os.LookupEnv("LOOPBACK"); ok {
		switch envVal {
		case "false", "0", "False", "FALSE":
			loopback = false
		case "true", "1", "True", "TRUE":
			loopback = true
		default:
			l.Warn("Invalid LOOPBACK value: %s, using flag/default: %v", envVal, loopback)
		}
	}

	l.Info("Using interface: %s", *iface)
	l.Info("Loopback filtering enabled: %v", loopback)

	eventChan := make(chan PayLoadTc, 10000)

	// Packet printing goroutine
	go func() {
		for evt := range eventChan {
			printPacket(evt, evt.Iface)
		}
	}()

	var collectAll bool = true
	var interfaces []net.Interface
	var err error
	//

	if collectAll == false {
		interfaces, err = interfaceCollector()
		if err != nil {
			l.Fatal("failed to get interface name")
		}
	} else {
		interfaces = append(interfaces, net.Interface{Name: *iface})
	}

	l.Info("getted interfaces : %v", interfaces)

	// Load eBPF
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
		l.Fatal("Unsupported architecture: %s", arch)
	}
	_, filename, _, _ := runtime.Caller(0)
	bpfPath := filepath.Join(filepath.Dir(filename), "../../bpf/network/build/tc-"+archDir+".o")

	spec, err := loadBpfSpec(bpfPath)
	if err != nil {
		l.Fatal("failed to load eBPF: %v", err)
	}

	var objs struct {
		TcIngress *ebpf.Program `ebpf:"tc_ingress"`
		TcEgress  *ebpf.Program `ebpf:"tc_egress"`
		Events    *ebpf.Map     `ebpf:"events"`
	}

	raiseMemlockLimit()
	if err := spec.LoadAndAssign(&objs, nil); err != nil {
		l.Fatal("eBPF load failed: %v", err)
	}
	defer objs.TcIngress.Close()
	defer objs.TcEgress.Close()
	defer objs.Events.Close()

	var wg sync.WaitGroup

	for _, iface := range interfaces {
		wg.Add(1)
		iface := iface // shadow copy for goroutine
		go func() {
			defer wg.Done()
			l.Info("Starting capture on %s", iface.Name)

			link, err := netlink.LinkByName(iface.Name)
			if err != nil {
				l.Warn("link not found: %v", err)
				return
			}

			// Add clsact if needed
			qdiscs, _ := netlink.QdiscList(link)
			clsactExists := false
			for _, q := range qdiscs {
				if q.Type() == "clsact" {
					clsactExists = true
					break
				}
			}
			if !clsactExists {
				_ = netlink.QdiscAdd(&netlink.GenericQdisc{
					QdiscAttrs: netlink.QdiscAttrs{
						LinkIndex: link.Attrs().Index,
						Handle:    netlink.MakeHandle(0xffff, 0),
						Parent:    netlink.HANDLE_CLSACT,
					},
					QdiscType: "clsact",
				})
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

			// Perf reader
			reader, err := perf.NewReader(objs.Events, os.Getpagesize())
			if err != nil {
				l.Warn("failed to create perf reader for %s: %v", iface.Name, err)
				return
			}
			defer reader.Close()

			for {
				select {
				case <-ctx.Done():
					l.Info("Stopping capture on %s", iface.Name)
					reader.Close()

					// Cleanup qdisc
					_ = netlink.QdiscDel(&netlink.GenericQdisc{
						QdiscAttrs: netlink.QdiscAttrs{
							LinkIndex: link.Attrs().Index,
							Handle:    netlink.MakeHandle(0xffff, 0),
							Parent:    netlink.HANDLE_CLSACT,
						},
						QdiscType: "clsact",
					})

					return

				default:
					record, err := reader.Read()
					if err != nil {
						if err == perf.ErrClosed {
							return
						}
						continue
					}

					if record.LostSamples > 0 {
						l.Warn("lost %d samples on %s", record.LostSamples, iface.Name)
						continue
					}

					var event Event
					if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
						l.Warn("decode error on %s: %v", iface.Name, err)
						continue
					}

					// it will drop localhost events
					if loopback == true {
						drop := shouldDrop(event)
						if drop == true {
							l.Info("event droped :%v", drop)
							continue
						}
					}

					payload := PayLoadTc{Iface: iface.Name, Event: event}
					select {
					case eventChan <- payload:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	// Wait for SIGINT
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	l.Info("Shutting down...")
	cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		l.Warn("Timeout waiting for goroutines to finish")
	}

	close(eventChan)
	l.Info("Shutdown complete")
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

func isLocalhost(ip uint32) bool {
	// Match exactly 127.0.0.1 in little-endian format
	return ip == 0x0100007F
}

func shouldDrop(event Event) bool {
	// Drop if the source is localhost (127.0.0.1)
	if isLocalhost(event.SrcIP) {
		return true
	}

	// Optionally drop system DNS stub (127.0.0.53)
	// 127.0.0.53 == 0x3500007F in little-endian
	// if event.SrcIP == 0x3500007F && event.SrcPort == 53 {
	// 	return true
	// }

	return false
}

func printPacket(event PayLoadTc, iface string) {
	direction := "Ingress"
	if event.Event.Direction == 1 {
		direction = "Egress"
	}

	srcIP := intToIP(event.Event.SrcIP)
	dstIP := intToIP(event.Event.DstIP)

	srcDomain := resolveIP(srcIP)
	dstDomain := resolveIP(dstIP)

	switch event.Event.Protocol {
	case 6: // TCP
		flags := tcpFlagsToString(event.Event.TcpFlags)
		fmt.Printf("%s TCP: src=%s (%s):%d -> dst=%s (%s):%d | proto=%d | flags=%s | iface = %s \n",
			direction,
			srcIP, srcDomain, event.Event.SrcPort,
			dstIP, dstDomain, event.Event.DstPort,
			event.Event.Protocol,
			flags,
			iface,
		)
	case 17: // UDP
		flags := tcpFlagsToString(event.Event.TcpFlags)
		fmt.Printf("%s UDP: src=%s (%s):%d -> dst=%s (%s):%d | proto=%d | flags= %s | iface = %s \n",
			direction,
			srcIP, srcDomain, event.Event.SrcPort,
			dstIP, dstDomain, event.Event.DstPort,
			event.Event.Protocol,
			flags,
			iface,
		)
	case 1: // ICMP
		flags := tcpFlagsToString(event.Event.TcpFlags)
		fmt.Printf("%s ICMP: src=%s (%s) -> dst=%s (%s) | proto=%d |flags = %s, iface = %s \n",
			direction,
			srcIP, srcDomain,
			dstIP, dstDomain,
			event.Event.Protocol,
			flags,
			iface,
		)
	default:
		flags := tcpFlagsToString(event.Event.TcpFlags)
		fmt.Printf("%s UNKNOWN: src=%s (%s) -> dst=%s (%s) | proto=%d | flags = %s , iface = %s\n",
			direction,
			srcIP, srcDomain,
			dstIP, dstDomain,
			event.Event.Protocol,
			flags,
			iface,
		)
	}
}

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
		l.Fatal("❌ Failed to raise rlimit: %v", err)
	}
}
