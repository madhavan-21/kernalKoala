// Required dependencies in go.mod:
// github.com/cilium/ebpf v0.12.3
// github.com/vishvananda/netlink v1.1.0
// github.com/miekg/dns v1.1.55
// golang.org/x/sys v0.13.0

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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/perf"
	"github.com/miekg/dns"
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

// Statistics for monitoring performance
type Stats struct {
	PacketsProcessed uint64
	PacketsDropped   uint64
	WorkerQueueFull  uint64
}

// Configuration for the capture system
type CaptureConfig struct {
	Interface      string
	LoopbackFilter bool
	WorkerCount    int
	BufferSize     int
	BatchSize      int
	EnableDNS      bool
	DNSTimeout     time.Duration
	DNSCacheSize   int
	DNSCacheTTL    time.Duration
	DNSServers     []string
}

// DNS Cache entry
type DNSCacheEntry struct {
	domain    string
	timestamp time.Time
	ttl       time.Duration
}

// High-performance DNS resolver with caching
type DNSResolver struct {
	client       *dns.Client
	servers      []string
	cache        sync.Map // map[string]*DNSCacheEntry
	enabled      bool
	timeout      time.Duration
	cacheTTL     time.Duration
	maxCacheSize int
	cacheCount   int64
	mu           sync.RWMutex
}

func NewDNSResolver(config *CaptureConfig) *DNSResolver {
	if !config.EnableDNS {
		return &DNSResolver{enabled: false}
	}

	resolver := &DNSResolver{
		client: &dns.Client{
			Timeout: config.DNSTimeout,
		},
		servers:      config.DNSServers,
		enabled:      true,
		timeout:      config.DNSTimeout,
		cacheTTL:     config.DNSCacheTTL,
		maxCacheSize: config.DNSCacheSize,
	}

	// Start cache cleanup goroutine
	go resolver.cleanupCache()

	return resolver
}

func (r *DNSResolver) cleanupCache() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		if !r.enabled {
			continue
		}

		now := time.Now()
		r.cache.Range(func(key, value interface{}) bool {
			entry := value.(*DNSCacheEntry)
			if now.Sub(entry.timestamp) > entry.ttl {
				r.cache.Delete(key)
				atomic.AddInt64(&r.cacheCount, -1)
			}
			return true
		})
	}
}

func (r *DNSResolver) ResolveIP(ip net.IP) string {
	if !r.enabled {
		return "-"
	}

	ipStr := ip.String()

	// Check cache first
	if cached, ok := r.cache.Load(ipStr); ok {
		entry := cached.(*DNSCacheEntry)
		if time.Since(entry.timestamp) < entry.ttl {
			return entry.domain
		}
		// Expired, remove from cache
		r.cache.Delete(ipStr)
		atomic.AddInt64(&r.cacheCount, -1)
	}

	// Don't resolve private/local IPs to reduce noise
	if r.isPrivateIP(ip) {
		return "-"
	}

	// Perform DNS lookup with timeout
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	domain := r.performLookup(ctx, ipStr)

	// Cache the result (even if it's "-" to avoid repeated failed lookups)
	if atomic.LoadInt64(&r.cacheCount) < int64(r.maxCacheSize) {
		entry := &DNSCacheEntry{
			domain:    domain,
			timestamp: time.Now(),
			ttl:       r.cacheTTL,
		}
		if _, loaded := r.cache.LoadOrStore(ipStr, entry); !loaded {
			atomic.AddInt64(&r.cacheCount, 1)
		}
	}

	return domain
}

func (r *DNSResolver) performLookup(ctx context.Context, ip string) string {
	// Use multiple DNS servers with fallback
	for _, server := range r.servers {
		select {
		case <-ctx.Done():
			return "-"
		default:
		}

		if domain := r.queryDNSServer(server, ip); domain != "-" {
			return domain
		}
	}

	// Fallback to system resolver as last resort
	names, err := net.LookupAddr(ip)
	if err != nil || len(names) == 0 {
		return "-"
	}
	return names[0]
}

func (r *DNSResolver) queryDNSServer(server, ip string) string {
	// Create reverse DNS query
	arpa, err := dns.ReverseAddr(ip)
	if err != nil {
		return "-"
	}

	msg := new(dns.Msg)
	msg.SetQuestion(arpa, dns.TypePTR)
	msg.RecursionDesired = true

	// Query DNS server
	resp, _, err := r.client.Exchange(msg, server)
	if err != nil || resp == nil || len(resp.Answer) == 0 {
		return "-"
	}

	// Extract PTR record
	for _, ans := range resp.Answer {
		if ptr, ok := ans.(*dns.PTR); ok {
			return ptr.Ptr
		}
	}

	return "-"
}

func (r *DNSResolver) isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Check for private IP ranges
	privateRanges := []struct {
		network *net.IPNet
	}{
		{parseIPNet("10.0.0.0/8")},
		{parseIPNet("172.16.0.0/12")},
		{parseIPNet("192.168.0.0/16")},
	}

	for _, private := range privateRanges {
		if private.network.Contains(ip) {
			return true
		}
	}

	return false
}

func parseIPNet(cidr string) *net.IPNet {
	_, network, _ := net.ParseCIDR(cidr)
	return network
}

type NetworkCapture struct {
	config      *CaptureConfig
	logger      *l.Logger
	stats       *Stats
	ctx         context.Context
	cancel      context.CancelFunc
	eventChan   chan PayLoadTc
	workerPool  chan chan PayLoadTc
	wg          sync.WaitGroup
	dnsResolver *DNSResolver
}

func NewNetworkCapture(config *CaptureConfig, logger *l.Logger) *NetworkCapture {
	ctx, cancel := context.WithCancel(context.Background())

	return &NetworkCapture{
		config:      config,
		logger:      logger,
		stats:       &Stats{},
		ctx:         ctx,
		cancel:      cancel,
		eventChan:   make(chan PayLoadTc, config.BufferSize),
		workerPool:  make(chan chan PayLoadTc, config.WorkerCount),
		dnsResolver: NewDNSResolver(config),
	}
}

func NetworkTrafficCapture(log *l.Logger) {
	// Parse configuration
	config := parseConfig()

	capture := NewNetworkCapture(config, log)
	defer capture.Shutdown()

	// Start worker pool
	capture.startWorkerPool()

	// Start packet dispatcher
	capture.startPacketDispatcher()

	// Start statistics reporter
	capture.startStatsReporter()

	// Get interfaces to monitor
	interfaces, err := capture.getInterfaces()
	if err != nil {
		log.Fatal("failed to get interfaces: %v", err)
	}

	// Load eBPF programs
	objs, err := capture.loadEBPF()
	if err != nil {
		log.Fatal("failed to load eBPF: %v", err)
	}
	defer capture.closeEBPF(objs)

	// Start capture on all interfaces
	for _, iface := range interfaces {
		capture.wg.Add(1)
		go capture.captureInterface(iface, objs)
	}

	// Wait for shutdown signal
	capture.waitForShutdown()
}

func parseConfig() *CaptureConfig {
	iface := flag.String("iface", "", "Network interface to attach (can also set IFACE env variable)")
	loopbackFlag := flag.Bool("loopback", true, "Set to false to allow localhost (loopback) traffic; default is true (drop loopback)")
	workers := flag.Int("workers", runtime.NumCPU(), "Number of worker goroutines for packet processing")
	bufferSize := flag.Int("buffer", 100000, "Event channel buffer size")
	batchSize := flag.Int("batch", 100, "Batch size for packet processing")
	enableDNS := flag.Bool("dns", true, "Enable DNS resolution for IP addresses")
	dnsTimeout := flag.Duration("dns-timeout", 500*time.Millisecond, "DNS query timeout")
	dnsCacheSize := flag.Int("dns-cache-size", 10000, "Maximum number of DNS cache entries")
	dnsCacheTTL := flag.Duration("dns-cache-ttl", 5*time.Minute, "DNS cache entry TTL")
	dnsServers := flag.String("dns-servers", "8.8.8.8:53,1.1.1.1:53", "Comma-separated list of DNS servers")
	flag.Parse()

	config := &CaptureConfig{
		WorkerCount:  *workers,
		BufferSize:   *bufferSize,
		BatchSize:    *batchSize,
		EnableDNS:    *enableDNS,
		DNSTimeout:   *dnsTimeout,
		DNSCacheSize: *dnsCacheSize,
		DNSCacheTTL:  *dnsCacheTTL,
	}

	// Parse DNS servers
	if *enableDNS {
		servers := []string{}
		for _, server := range []string{"8.8.8.8:53", "1.1.1.1:53", "208.67.222.222:53"} {
			if *dnsServers != "" {
				// Parse custom DNS servers
				for _, s := range splitString(*dnsServers, ",") {
					if s != "" {
						servers = append(servers, s)
					}
				}
				break
			}
			servers = append(servers, server)
		}
		config.DNSServers = servers
	}

	// Resolve interface
	if *iface == "" {
		if envIface := os.Getenv("IFACE"); envIface != "" {
			config.Interface = envIface
		} else {
			config.Interface = "lo"
		}
	} else {
		config.Interface = *iface
	}

	// Resolve loopback filtering
	config.LoopbackFilter = *loopbackFlag
	if envVal, ok := os.LookupEnv("LOOPBACK"); ok {
		switch envVal {
		case "false", "0", "False", "FALSE":
			config.LoopbackFilter = false
		case "true", "1", "True", "TRUE":
			config.LoopbackFilter = true
		}
	}

	return config
}

func splitString(s, sep string) []string {
	var result []string
	if s == "" {
		return result
	}
	parts := make([]string, 0)
	current := ""
	for _, char := range s {
		if string(char) == sep {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

func (nc *NetworkCapture) startWorkerPool() {
	// Create worker pool
	for i := 0; i < nc.config.WorkerCount; i++ {
		worker := &PacketWorker{
			id:          i,
			workerPool:  nc.workerPool,
			jobChan:     make(chan PayLoadTc, nc.config.BatchSize),
			logger:      nc.logger,
			stats:       nc.stats,
			dnsResolver: nc.dnsResolver,
		}
		go worker.start(nc.ctx)
	}

	nc.logger.Info("Started %d worker goroutines", nc.config.WorkerCount)
}

func (nc *NetworkCapture) startPacketDispatcher() {
	go func() {
		for {
			select {
			case <-nc.ctx.Done():
				return
			case event := <-nc.eventChan:
				// Try to get an available worker
				select {
				case worker := <-nc.workerPool:
					select {
					case worker <- event:
					default:
						// Worker queue is full, drop packet
						atomic.AddUint64(&nc.stats.WorkerQueueFull, 1)
						atomic.AddUint64(&nc.stats.PacketsDropped, 1)
						// Put worker back
						select {
						case nc.workerPool <- worker:
						default:
						}
					}
				default:
					// No workers available, drop packet
					atomic.AddUint64(&nc.stats.PacketsDropped, 1)
				}
			}
		}
	}()
}

func (nc *NetworkCapture) startStatsReporter() {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-nc.ctx.Done():
				return
			case <-ticker.C:
				processed := atomic.LoadUint64(&nc.stats.PacketsProcessed)
				dropped := atomic.LoadUint64(&nc.stats.PacketsDropped)
				queueFull := atomic.LoadUint64(&nc.stats.WorkerQueueFull)

				nc.logger.Info("Stats - Processed: %d, Dropped: %d, Queue Full: %d",
					processed, dropped, queueFull)
			}
		}
	}()
}

type PacketWorker struct {
	id          int
	workerPool  chan chan PayLoadTc
	jobChan     chan PayLoadTc
	logger      *l.Logger
	stats       *Stats
	dnsResolver *DNSResolver
}

func (w *PacketWorker) start(ctx context.Context) {
	// Pre-allocate batch slice
	batch := make([]PayLoadTc, 0, 100)
	ticker := time.NewTicker(10 * time.Millisecond) // Process batches every 10ms
	defer ticker.Stop()

	for {
		// Register this worker in the pool
		w.workerPool <- w.jobChan

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if len(batch) > 0 {
				w.processBatch(batch)
				batch = batch[:0] // Reset slice but keep capacity
			}
		case job := <-w.jobChan:
			batch = append(batch, job)

			// Process batch when it's full
			if len(batch) >= cap(batch) {
				w.processBatch(batch)
				batch = batch[:0] // Reset slice but keep capacity
			}
		}
	}
}

func (w *PacketWorker) processBatch(batch []PayLoadTc) {
	for _, event := range batch {
		w.printPacket(event)
		atomic.AddUint64(&w.stats.PacketsProcessed, 1)
	}
}

func (w *PacketWorker) printPacket(event PayLoadTc) {
	direction := "Ingress"
	if event.Event.Direction == 1 {
		direction = "Egress"
	}

	srcIP := intToIP(event.Event.SrcIP)
	dstIP := intToIP(event.Event.DstIP)

	// Resolve DNS names if enabled
	var srcDomain, dstDomain string
	if w.dnsResolver.enabled {
		srcDomain = w.dnsResolver.ResolveIP(srcIP)
		dstDomain = w.dnsResolver.ResolveIP(dstIP)
	} else {
		srcDomain = "-"
		dstDomain = "-"
	}

	// Use a more efficient string builder approach
	var output string
	flags := tcpFlagsToString(event.Event.TcpFlags)

	switch event.Event.Protocol {
	case 6: // TCP
		output = fmt.Sprintf("%s TCP: src=%s(%s):%d -> dst=%s(%s):%d | flags=%s | iface=%s",
			direction, srcIP, srcDomain, event.Event.SrcPort,
			dstIP, dstDomain, event.Event.DstPort, flags, event.Iface)
	case 17: // UDP
		output = fmt.Sprintf("%s UDP: src=%s(%s):%d -> dst=%s(%s):%d | flags=%s | iface=%s",
			direction, srcIP, srcDomain, event.Event.SrcPort,
			dstIP, dstDomain, event.Event.DstPort, flags, event.Iface)
	case 1: // ICMP
		output = fmt.Sprintf("%s ICMP: src=%s(%s) -> dst=%s(%s) | flags=%s | iface=%s",
			direction, srcIP, srcDomain, dstIP, dstDomain, flags, event.Iface)
	default:
		output = fmt.Sprintf("%s PROTO_%d: src=%s(%s) -> dst=%s(%s) | flags=%s | iface=%s",
			direction, event.Event.Protocol, srcIP, srcDomain,
			dstIP, dstDomain, flags, event.Iface)
	}

	fmt.Println(output)
}

func (nc *NetworkCapture) getInterfaces() ([]net.Interface, error) {
	var interfaces []net.Interface

	// For now, just use the specified interface
	// You can extend this to collect all interfaces if needed
	interfaces = append(interfaces, net.Interface{Name: nc.config.Interface})

	nc.logger.Info("Monitoring interfaces: %v", interfaces)
	return interfaces, nil
}

func (nc *NetworkCapture) loadEBPF() (*EBPFObjects, error) {
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
		return nil, fmt.Errorf("unsupported architecture: %s", arch)
	}

	_, filename, _, _ := runtime.Caller(0)
	bpfPath := filepath.Join(filepath.Dir(filename), "../../bpf/network/build/tc-"+archDir+".o")

	spec, err := loadBpfSpec(bpfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load eBPF spec: %v", err)
	}

	objs := &EBPFObjects{}

	raiseMemlockLimit()
	if err := spec.LoadAndAssign(objs, nil); err != nil {
		return nil, fmt.Errorf("eBPF load failed: %v", err)
	}

	return objs, nil
}

type EBPFObjects struct {
	TcIngress *ebpf.Program `ebpf:"tc_ingress"`
	TcEgress  *ebpf.Program `ebpf:"tc_egress"`
	Events    *ebpf.Map     `ebpf:"events"`
}

func (nc *NetworkCapture) closeEBPF(objs *EBPFObjects) {
	if objs.TcIngress != nil {
		objs.TcIngress.Close()
	}
	if objs.TcEgress != nil {
		objs.TcEgress.Close()
	}
	if objs.Events != nil {
		objs.Events.Close()
	}
}

func (nc *NetworkCapture) captureInterface(iface net.Interface, objs *EBPFObjects) {
	defer nc.wg.Done()
	nc.logger.Info("Starting capture on %s", iface.Name)

	link, err := netlink.LinkByName(iface.Name)
	if err != nil {
		nc.logger.Warn("link not found: %v", err)
		return
	}

	// Setup TC filters
	if err := nc.setupTCFilters(link, objs); err != nil {
		nc.logger.Warn("failed to setup TC filters: %v", err)
		return
	}
	defer nc.cleanupTCFilters(link)

	// Create perf reader with larger buffer
	reader, err := perf.NewReader(objs.Events, os.Getpagesize()*16) // Larger buffer
	if err != nil {
		nc.logger.Warn("failed to create perf reader for %s: %v", iface.Name, err)
		return
	}
	defer reader.Close()

	// Process events
	for {
		select {
		case <-nc.ctx.Done():
			nc.logger.Info("Stopping capture on %s", iface.Name)
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
				nc.logger.Warn("lost %d samples on %s", record.LostSamples, iface.Name)
				atomic.AddUint64(&nc.stats.PacketsDropped, record.LostSamples)
				continue
			}

			var event Event
			if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
				nc.logger.Warn("decode error on %s: %v", iface.Name, err)
				continue
			}

			// Apply loopback filter
			if nc.config.LoopbackFilter && shouldDrop(event) {
				continue
			}

			payload := PayLoadTc{Iface: iface.Name, Event: event}

			// Non-blocking send to event channel
			select {
			case nc.eventChan <- payload:
			default:
				// Channel is full, drop packet
				atomic.AddUint64(&nc.stats.PacketsDropped, 1)
			}
		}
	}
}

func (nc *NetworkCapture) setupTCFilters(link netlink.Link, objs *EBPFObjects) error {
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
		if err := netlink.QdiscAdd(&netlink.GenericQdisc{
			QdiscAttrs: netlink.QdiscAttrs{
				LinkIndex: link.Attrs().Index,
				Handle:    netlink.MakeHandle(0xffff, 0),
				Parent:    netlink.HANDLE_CLSACT,
			},
			QdiscType: "clsact",
		}); err != nil {
			return err
		}
	}

	// Attach ingress filter
	if err := netlink.FilterAdd(&netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: link.Attrs().Index,
			Parent:    netlink.HANDLE_MIN_INGRESS,
			Handle:    netlink.MakeHandle(0, 1),
			Protocol:  syscall.ETH_P_ALL,
		},
		Fd:           objs.TcIngress.FD(),
		Name:         "tc_ingress",
		DirectAction: true,
	}); err != nil {
		return err
	}

	// Attach egress filter
	if err := netlink.FilterAdd(&netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: link.Attrs().Index,
			Parent:    netlink.HANDLE_MIN_EGRESS,
			Handle:    netlink.MakeHandle(0, 1),
			Protocol:  syscall.ETH_P_ALL,
		},
		Fd:           objs.TcEgress.FD(),
		Name:         "tc_egress",
		DirectAction: true,
	}); err != nil {
		return err
	}

	return nil
}

func (nc *NetworkCapture) cleanupTCFilters(link netlink.Link) {
	netlink.QdiscDel(&netlink.GenericQdisc{
		QdiscAttrs: netlink.QdiscAttrs{
			LinkIndex: link.Attrs().Index,
			Handle:    netlink.MakeHandle(0xffff, 0),
			Parent:    netlink.HANDLE_CLSACT,
		},
		QdiscType: "clsact",
	})
}

func (nc *NetworkCapture) waitForShutdown() {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	nc.logger.Info("Shutting down...")
	nc.cancel()

	done := make(chan struct{})
	go func() {
		nc.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		nc.logger.Warn("Timeout waiting for goroutines to finish")
	}
}

func (nc *NetworkCapture) Shutdown() {
	nc.cancel()
	close(nc.eventChan)
	nc.logger.Info("Shutdown complete")
}

// Helper functions remain the same
func loadBpfSpec(path string) (*ebpf.CollectionSpec, error) {
	spec, err := ebpf.LoadCollectionSpec(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load BPF spec: %v", err)
	}

	if eventMap, ok := spec.Maps["events"]; ok {
		eventMap.Type = ebpf.PerfEventArray
	}
	return spec, nil
}

func isLocalhost(ip uint32) bool {
	return ip == 0x0100007F // 127.0.0.1 in little-endian
}

func shouldDrop(event Event) bool {
	return isLocalhost(event.SrcIP)
}

func tcpFlagsToString(flags uint8) string {
	if flags == 0 {
		return "NONE"
	}

	flagNames := []struct {
		mask uint8
		name string
	}{
		{0x01, "FIN"}, {0x02, "SYN"}, {0x04, "RST"}, {0x08, "PSH"},
		{0x10, "ACK"}, {0x20, "URG"}, {0x40, "ECE"}, {0x80, "CWR"},
	}

	var result []string
	for _, f := range flagNames {
		if flags&f.mask != 0 {
			result = append(result, f.name)
		}
	}

	return fmt.Sprintf("0x%02x(%v)", flags, result)
}

func intToIP(ip uint32) net.IP {
	return net.IPv4(byte(ip), byte(ip>>8), byte(ip>>16), byte(ip>>24))
}

func raiseMemlockLimit() {
	rLimit := &unix.Rlimit{
		Cur: unix.RLIM_INFINITY,
		Max: unix.RLIM_INFINITY,
	}
	if err := unix.Setrlimit(unix.RLIMIT_MEMLOCK, rLimit); err != nil {
		l.Fatal("❌ Failed to raise rlimit: %v", err)
	}
}
