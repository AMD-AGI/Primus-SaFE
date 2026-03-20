package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -D__TARGET_ARCH_x86" -target amd64 tracer ../bpf/tracer.bpf.c -- -I/usr/include -I../bpf

const (
	EvtConnectEnter    = 1
	EvtConnectExit     = 2
	EvtAcceptExit      = 3
	EvtTCPState        = 4
	EvtTCPRetransmit   = 5
	EvtTCPReset        = 6
	EvtSendmsgSlow     = 7
	EvtRecvmsgSlow     = 8
	EvtClose           = 9
	EvtAnpConnect      = 20
	EvtAnpConnectRet   = 21
	EvtAnpAccept       = 22
	EvtAnpAcceptRet    = 23
	EvtAnpIsend        = 24
	EvtAnpIrecv        = 25
	EvtAnpCloseSend    = 26
	EvtAnpCloseRecv    = 27
	EvtIbvCreateQP     = 30
	EvtIbvModifyQP     = 31
	EvtIbvModifyQPRet  = 32
)

var eventNames = map[uint32]string{
	EvtConnectEnter:   "CONNECT",
	EvtConnectExit:    "CONNECT_DONE",
	EvtAcceptExit:     "ACCEPT_DONE",
	EvtTCPState:       "TCP_STATE",
	EvtTCPRetransmit:  "TCP_RETX",
	EvtTCPReset:       "TCP_RESET",
	EvtSendmsgSlow:    "SEND_SLOW",
	EvtRecvmsgSlow:    "RECV_SLOW",
	EvtClose:          "CLOSE",
	EvtAnpConnect:     "ANP_CONNECT",
	EvtAnpConnectRet:  "ANP_CONNECT_RET",
	EvtAnpAccept:      "ANP_ACCEPT",
	EvtAnpAcceptRet:   "ANP_ACCEPT_RET",
	EvtAnpIsend:       "ANP_ISEND",
	EvtAnpIrecv:       "ANP_IRECV",
	EvtAnpCloseSend:   "ANP_CLOSE_SEND",
	EvtAnpCloseRecv:   "ANP_CLOSE_RECV",
	EvtIbvCreateQP:    "IBV_CREATE_QP",
	EvtIbvModifyQP:    "IBV_MODIFY_QP",
	EvtIbvModifyQPRet: "IBV_MODIFY_QP_RET",
}

var tcpStateNames = map[uint32]string{
	1:  "ESTABLISHED",
	2:  "SYN_SENT",
	3:  "SYN_RECV",
	4:  "FIN_WAIT1",
	5:  "FIN_WAIT2",
	6:  "TIME_WAIT",
	7:  "CLOSE",
	8:  "CLOSE_WAIT",
	9:  "LAST_ACK",
	10: "LISTEN",
}

type Event struct {
	TsNs       uint64
	Pid        uint32
	Tid        uint32
	EventType  uint32
	AF         uint32
	Sport      uint32
	Dport      uint32
	Saddr      [16]byte
	Daddr      [16]byte
	DurationNs int64
	Retval     int32
	OldState   uint32
	NewState   uint32
	Comm       [16]byte
}

type ContainerInfo struct {
	Name        string
	ContainerID string
	Namespace   string
	PodName     string
}

var (
	outputDir      string
	containerCache sync.Map // host PID -> ContainerInfo
	fileCache      sync.Map // "container/pid" -> *os.File
	nsFilter       string
)

func main() {
	outputDir = getEnv("TRACE_OUTPUT_DIR", "/var/log/rccl-tracer")
	nsFilter = getEnv("TRACE_NAMESPACE_FILTER", "control-plane-")
	slowThresholdMs, _ := strconv.ParseInt(getEnv("TRACE_SLOW_THRESHOLD_MS", "100"), 10, 64)

	os.MkdirAll(outputDir, 0755)

	spec, err := loadTracer()
	if err != nil {
		log.Fatalf("Failed to load BPF spec: %v", err)
	}

	var objs tracerObjects
	if err := spec.LoadAndAssign(&objs, nil); err != nil {
		log.Fatalf("Failed to load BPF objects: %v", err)
	}
	defer objs.Close()

	// Set slow I/O threshold
	thresholdNs := uint64(slowThresholdMs) * 1_000_000
	key := uint32(0)
	if err := objs.TracerConfig.Put(key, thresholdNs); err != nil {
		log.Printf("Warning: failed to set config: %v", err)
	}

	// Set cgroup filter: key=0 value=1 means "trace all, filter in userspace"
	cgKey := uint64(0)
	cgVal := uint8(1)
	if err := objs.CgroupFilter.Put(cgKey, cgVal); err != nil {
		log.Printf("Warning: failed to set cgroup filter: %v", err)
	}

	var links []link.Link
	tryAttach := func(name string, fn func() (link.Link, error)) {
		l, err := fn()
		if err != nil {
			log.Printf("Warning: failed to attach %s: %v", name, err)
			return
		}
		links = append(links, l)
		log.Printf("Attached: %s", name)
	}

	// Layer 1: TCP lifecycle (kprobe only, tracepoints have cross-kernel struct issues)
	tryAttach("tcp_reset", func() (link.Link, error) {
		return link.Kprobe("tcp_reset", objs.HandleTcpReset, nil)
	})

	// Layer 2: Syscall latency
	tryAttach("sys_enter_connect", func() (link.Link, error) {
		return link.Tracepoint("syscalls", "sys_enter_connect", objs.HandleConnectEnter, nil)
	})
	tryAttach("sys_exit_connect", func() (link.Link, error) {
		return link.Tracepoint("syscalls", "sys_exit_connect", objs.HandleConnectExit, nil)
	})
	tryAttach("sys_enter_accept4", func() (link.Link, error) {
		return link.Tracepoint("syscalls", "sys_enter_accept4", objs.HandleAcceptEnter, nil)
	})
	tryAttach("sys_exit_accept4", func() (link.Link, error) {
		return link.Tracepoint("syscalls", "sys_exit_accept4", objs.HandleAcceptExit, nil)
	})
	tryAttach("sys_enter_sendto", func() (link.Link, error) {
		return link.Tracepoint("syscalls", "sys_enter_sendto", objs.HandleSendtoEnter, nil)
	})
	tryAttach("sys_exit_sendto", func() (link.Link, error) {
		return link.Tracepoint("syscalls", "sys_exit_sendto", objs.HandleSendtoExit, nil)
	})
	tryAttach("sys_enter_recvfrom", func() (link.Link, error) {
		return link.Tracepoint("syscalls", "sys_enter_recvfrom", objs.HandleRecvfromEnter, nil)
	})
	tryAttach("sys_exit_recvfrom", func() (link.Link, error) {
		return link.Tracepoint("syscalls", "sys_exit_recvfrom", objs.HandleRecvfromExit, nil)
	})

	// Layer 3: Dynamic uprobe discovery for RCCL ANP + ibverbs
	// Libraries live inside training containers, so we scan /host/proc/<pid>/root/
	// to find them once a training process appears on this node.
	var linksMu sync.Mutex
	uprobesAttached := false

	anpRelPath := getEnv("ANP_LIB_REL_PATH", "/opt/rocm-7.1.0/lib/librccl-anp.so")
	ibvRelPath := getEnv("IBV_LIB_REL_PATH", "/lib/x86_64-linux-gnu/libibverbs.so.1")

	go func() {
		scanInterval := 5 * time.Second
		for {
			if uprobesAttached {
				return
			}
			time.Sleep(scanInterval)

			pid := findTrainingPid(anpRelPath)
			if pid == 0 {
				continue
			}

			hostAnpPath := fmt.Sprintf("/host/proc/%d/root%s", pid, anpRelPath)
			hostIbvPath := fmt.Sprintf("/host/proc/%d/root%s", pid, ibvRelPath)

			if _, err := os.Stat(hostAnpPath); err != nil {
				continue
			}

			log.Printf("Found training process PID %d, attaching L3 uprobes via %s", pid, hostAnpPath)

			attachU := func(name, lib, symbol string, prog *ebpf.Program) {
				ex, err := link.OpenExecutable(lib)
				if err != nil {
					log.Printf("Warning: uprobe %s open %s: %v", name, filepath.Base(lib), err)
					return
				}
				l, err := ex.Uprobe(symbol, prog, nil)
				if err != nil {
					log.Printf("Warning: uprobe %s (%s): %v", name, symbol, err)
					return
				}
				linksMu.Lock()
				links = append(links, l)
				linksMu.Unlock()
				log.Printf("Attached uprobe: %s → %s:%s", name, filepath.Base(lib), symbol)
			}

			attachUR := func(name, lib, symbol string, prog *ebpf.Program) {
				ex, err := link.OpenExecutable(lib)
				if err != nil {
					log.Printf("Warning: uretprobe %s open %s: %v", name, filepath.Base(lib), err)
					return
				}
				l, err := ex.Uretprobe(symbol, prog, nil)
				if err != nil {
					log.Printf("Warning: uretprobe %s (%s): %v", name, symbol, err)
					return
				}
				linksMu.Lock()
				links = append(links, l)
				linksMu.Unlock()
				log.Printf("Attached uretprobe: %s → %s:%s", name, filepath.Base(lib), symbol)
			}

			// librccl-anp.so
			attachU("anp_connect", hostAnpPath, "anpNetConnect", objs.HandleAnpConnect)
			attachUR("anp_connect_ret", hostAnpPath, "anpNetConnect", objs.HandleAnpConnectRet)
			attachU("anp_accept", hostAnpPath, "anpNetAccept", objs.HandleAnpAccept)
			attachUR("anp_accept_ret", hostAnpPath, "anpNetAccept", objs.HandleAnpAcceptRet)
			attachU("anp_isend", hostAnpPath, "anpNetIsend", objs.HandleAnpIsend)
			attachU("anp_irecv", hostAnpPath, "anpNetIrecv", objs.HandleAnpIrecv)
			attachU("anp_close_send", hostAnpPath, "anpNetCloseSend", objs.HandleAnpCloseSend)
			attachU("anp_close_recv", hostAnpPath, "anpNetCloseRecv", objs.HandleAnpCloseRecv)

			// libibverbs.so
			if _, err := os.Stat(hostIbvPath); err == nil {
				attachU("ibv_create_qp", hostIbvPath, "ibv_create_qp", objs.HandleIbvCreateQp)
				attachU("ibv_modify_qp", hostIbvPath, "ibv_modify_qp", objs.HandleIbvModifyQp)
				attachUR("ibv_modify_qp_ret", hostIbvPath, "ibv_modify_qp", objs.HandleIbvModifyQpRet)
			}

			uprobesAttached = true
			log.Printf("Layer 3 uprobes attached successfully via PID %d", pid)
			return
		}
	}()

	defer func() {
		linksMu.Lock()
		for _, l := range links {
			l.Close()
		}
		linksMu.Unlock()
	}()

	log.Printf("rccl-socket-tracer running. Output: %s, namespace filter: %q, slow threshold: %dms",
		outputDir, nsFilter, slowThresholdMs)

	rd, err := ringbuf.NewReader(objs.Events)
	if err != nil {
		log.Fatalf("Failed to open ring buffer: %v", err)
	}
	defer rd.Close()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Println("Shutting down...")
		rd.Close()
	}()

	bootTime := getBootTime()

	for {
		record, err := rd.Read()
		if err != nil {
			if err == ringbuf.ErrClosed {
				break
			}
			log.Printf("ringbuf read error: %v", err)
			continue
		}

		var evt Event
		if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &evt); err != nil {
			continue
		}

		cinfo := resolveContainer(evt.Pid)
		if cinfo == nil {
			continue
		}
		if nsFilter != "" && !strings.Contains(cinfo.Namespace, nsFilter) {
			continue
		}

		wallTime := bootTime.Add(time.Duration(evt.TsNs))
		line := formatEvent(&evt, cinfo, wallTime)
		writeToFile(cinfo, evt.Pid, line)
	}

	// Flush all files
	fileCache.Range(func(_, v interface{}) bool {
		if f, ok := v.(*os.File); ok {
			f.Close()
		}
		return true
	})
}

func formatEvent(e *Event, c *ContainerInfo, ts time.Time) string {
	evtName := eventNames[e.EventType]
	comm := strings.TrimRight(string(e.Comm[:]), "\x00")
	timeStr := ts.Format("15:04:05.000")

	base := fmt.Sprintf("[%s] %-14s pid=%d tid=%d comm=%s",
		timeStr, evtName, e.Pid, e.Tid, comm)

	switch e.EventType {
	case EvtConnectEnter:
		dst := formatAddr(e.Daddr[:], e.AF)
		return fmt.Sprintf("%s dst=%s:%d", base, dst, e.Dport)

	case EvtConnectExit:
		durMs := float64(e.DurationNs) / 1e6
		errStr := ""
		if e.Retval < 0 {
			errStr = fmt.Sprintf(" errno=%d(%s)", -e.Retval, errnoName(-e.Retval))
		} else if e.Retval == 0 {
			errStr = " OK"
		} else {
			errStr = fmt.Sprintf(" ret=%d(EINPROGRESS)", e.Retval)
		}
		return fmt.Sprintf("%s duration=%.1fms%s", base, durMs, errStr)

	case EvtAcceptExit:
		durMs := float64(e.DurationNs) / 1e6
		if e.Retval < 0 {
			return fmt.Sprintf("%s duration=%.1fms errno=%d(%s)", base, durMs, -e.Retval, errnoName(-e.Retval))
		}
		return fmt.Sprintf("%s duration=%.1fms fd=%d", base, durMs, e.Retval)

	case EvtTCPState:
		src := formatAddr(e.Saddr[:], e.AF)
		dst := formatAddr(e.Daddr[:], e.AF)
		oldS := tcpStateNames[e.OldState]
		newS := tcpStateNames[e.NewState]
		return fmt.Sprintf("%s %s:%d→%s:%d %s→%s",
			base, src, e.Sport, dst, e.Dport, oldS, newS)

	case EvtTCPRetransmit:
		src := formatAddr(e.Saddr[:], e.AF)
		dst := formatAddr(e.Daddr[:], e.AF)
		return fmt.Sprintf("%s %s:%d→%s:%d", base, src, e.Sport, dst, e.Dport)

	case EvtTCPReset:
		src := formatAddr(e.Saddr[:], e.AF)
		dst := formatAddr(e.Daddr[:], e.AF)
		return fmt.Sprintf("%s %s:%d→%s:%d", base, src, e.Sport, dst, e.Dport)

	case EvtSendmsgSlow, EvtRecvmsgSlow:
		durMs := float64(e.DurationNs) / 1e6
		return fmt.Sprintf("%s duration=%.1fms ret=%d", base, durMs, e.Retval)

	case EvtAnpConnect:
		return fmt.Sprintf("%s dev=%d", base, e.Retval)
	case EvtAnpConnectRet:
		durMs := float64(e.DurationNs) / 1e6
		ncclErr := ncclResultName(e.Retval)
		return fmt.Sprintf("%s duration=%.1fms result=%s", base, durMs, ncclErr)
	case EvtAnpAccept:
		return base
	case EvtAnpAcceptRet:
		durMs := float64(e.DurationNs) / 1e6
		ncclErr := ncclResultName(e.Retval)
		return fmt.Sprintf("%s duration=%.1fms result=%s", base, durMs, ncclErr)
	case EvtAnpIsend:
		return fmt.Sprintf("%s size=%d tag=%d", base, e.DurationNs, e.Retval)
	case EvtAnpIrecv:
		return fmt.Sprintf("%s nbufs=%d", base, e.Retval)
	case EvtAnpCloseSend, EvtAnpCloseRecv:
		return base
	case EvtIbvCreateQP:
		return base
	case EvtIbvModifyQP:
		return base
	case EvtIbvModifyQPRet:
		durMs := float64(e.DurationNs) / 1e6
		if e.Retval != 0 {
			return fmt.Sprintf("%s duration=%.1fms FAILED ret=%d", base, durMs, e.Retval)
		}
		return fmt.Sprintf("%s duration=%.1fms OK", base, durMs)
	}

	return base
}

func formatAddr(addr []byte, af uint32) string {
	if af == 2 { // AF_INET
		return net.IP(addr[:4]).String()
	}
	return net.IP(addr[:16]).String()
}

func ncclResultName(r int32) string {
	switch r {
	case 0:
		return "ncclSuccess"
	case 1:
		return "ncclUnhandledCudaError"
	case 2:
		return "ncclSystemError"
	case 3:
		return "ncclInternalError"
	case 4:
		return "ncclInvalidArgument"
	case 5:
		return "ncclInvalidUsage"
	case 6:
		return "ncclRemoteError"
	case 7:
		return "ncclInProgress"
	default:
		return fmt.Sprintf("ncclError(%d)", r)
	}
}

func errnoName(e int32) string {
	switch e {
	case 111:
		return "ECONNREFUSED"
	case 110:
		return "ETIMEDOUT"
	case 104:
		return "ECONNRESET"
	case 115:
		return "EINPROGRESS"
	case 113:
		return "EHOSTUNREACH"
	case 101:
		return "ENETUNREACH"
	case 99:
		return "EADDRNOTAVAIL"
	case 98:
		return "EADDRINUSE"
	default:
		return fmt.Sprintf("E%d", e)
	}
}

// resolveContainer reads /proc/<pid>/cgroup to find container ID,
// then looks up container name via /proc/<pid>/environ or CRI metadata
func resolveContainer(hostPid uint32) *ContainerInfo {
	if cached, ok := containerCache.Load(hostPid); ok {
		return cached.(*ContainerInfo)
	}

	cgroupPath := fmt.Sprintf("/host/proc/%d/cgroup", hostPid)
	data, err := os.ReadFile(cgroupPath)
	if err != nil {
		return nil
	}

	info := parseCgroupForK8s(string(data), hostPid)
	if info == nil {
		return nil
	}

	containerCache.Store(hostPid, info)
	return info
}

func parseCgroupForK8s(cgroupData string, hostPid uint32) *ContainerInfo {
	// Look for kubepods cgroup path:
	// e.g. 0::/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod<UID>.slice/cri-containerd-<CID>.scope
	for _, line := range strings.Split(cgroupData, "\n") {
		if !strings.Contains(line, "kubepods") {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}
		path := parts[2]

		containerID := ""
		if idx := strings.LastIndex(path, "cri-containerd-"); idx >= 0 {
			cid := path[idx+len("cri-containerd-"):]
			cid = strings.TrimSuffix(cid, ".scope")
			containerID = cid
		} else if idx := strings.LastIndex(path, "docker-"); idx >= 0 {
			cid := path[idx+len("docker-"):]
			cid = strings.TrimSuffix(cid, ".scope")
			containerID = cid
		}

		// Extract pod UID from cgroup path
		podUID := ""
		if idx := strings.Index(path, "pod"); idx >= 0 {
			rest := path[idx+3:]
			if dotIdx := strings.Index(rest, "."); dotIdx >= 0 {
				podUID = rest[:dotIdx]
			} else if slashIdx := strings.Index(rest, "/"); slashIdx >= 0 {
				podUID = rest[:slashIdx]
			}
		}

		// Read container hostname from /proc/<pid>/environ for pod name
		podName := readPodName(hostPid)
		ns := readPodNamespace(hostPid)

		if containerID == "" && podUID == "" {
			continue
		}

		return &ContainerInfo{
			Name:        podName,
			ContainerID: containerID[:min(12, len(containerID))],
			Namespace:   ns,
			PodName:     podName,
		}
	}
	return nil
}

func readPodName(hostPid uint32) string {
	// Try reading HOSTNAME from /proc/<pid>/environ
	envPath := fmt.Sprintf("/host/proc/%d/environ", hostPid)
	data, err := os.ReadFile(envPath)
	if err != nil {
		return fmt.Sprintf("pid-%d", hostPid)
	}
	for _, entry := range bytes.Split(data, []byte{0}) {
		if bytes.HasPrefix(entry, []byte("HOSTNAME=")) {
			return string(entry[9:])
		}
	}
	return fmt.Sprintf("pid-%d", hostPid)
}

func readPodNamespace(hostPid uint32) string {
	envPath := fmt.Sprintf("/host/proc/%d/environ", hostPid)
	data, err := os.ReadFile(envPath)
	if err != nil {
		return "unknown"
	}
	// K8s injects POD_NAMESPACE via downward API; if not available, parse from cgroup
	for _, entry := range bytes.Split(data, []byte{0}) {
		if bytes.HasPrefix(entry, []byte("POD_NAMESPACE=")) {
			return string(entry[14:])
		}
	}

	// Fallback: read from /proc/<pid>/cgroup and check if it contains a known namespace
	cgPath := fmt.Sprintf("/host/proc/%d/cgroup", hostPid)
	cg, err := os.ReadFile(cgPath)
	if err != nil {
		return "unknown"
	}
	cgStr := string(cg)
	// Try to find namespace from pod labels file
	// Fallback: just return "unknown" and let userspace match by pod name
	_ = cgStr
	return "unknown"
}

func writeToFile(c *ContainerInfo, hostPid uint32, line string) {
	// Get container PID (PID 1 namespace mapping)
	containerPid := resolveContainerPid(hostPid)

	dirName := fmt.Sprintf("%s/%s", outputDir, c.PodName)
	os.MkdirAll(dirName, 0755)

	key := fmt.Sprintf("%s/%d", c.PodName, containerPid)

	var f *os.File
	if cached, ok := fileCache.Load(key); ok {
		f = cached.(*os.File)
	} else {
		fileName := filepath.Join(dirName, fmt.Sprintf("trace-pid%d.log", containerPid))
		var err error
		f, err = os.OpenFile(fileName, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			log.Printf("Failed to open %s: %v", fileName, err)
			return
		}
		fileCache.Store(key, f)

		// Write header
		fmt.Fprintf(f, "# rccl-socket-tracer\n")
		fmt.Fprintf(f, "# pod=%s namespace=%s container_id=%s\n",
			c.PodName, c.Namespace, c.ContainerID)
		fmt.Fprintf(f, "# host_pid=%d container_pid=%d\n", hostPid, containerPid)
		fmt.Fprintf(f, "# started=%s\n\n", time.Now().Format(time.RFC3339))
	}

	fmt.Fprintln(f, line)
}

func resolveContainerPid(hostPid uint32) uint32 {
	// Read /proc/<hostPid>/status and find NSpid line
	statusPath := fmt.Sprintf("/host/proc/%d/status", hostPid)
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return hostPid
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "NSpid:") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				// fields[1] = host PID, fields[2] = container PID
				if pid, err := strconv.ParseUint(fields[2], 10, 32); err == nil {
					return uint32(pid)
				}
			}
			if len(fields) >= 2 {
				if pid, err := strconv.ParseUint(fields[1], 10, 32); err == nil {
					return uint32(pid)
				}
			}
		}
	}
	return hostPid
}

// findTrainingPid scans /host/proc/ for a process whose root filesystem
// contains the given library path. Returns the first matching host PID.
func findTrainingPid(libRelPath string) uint32 {
	entries, err := os.ReadDir("/host/proc")
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid := entry.Name()
		if pid[0] < '1' || pid[0] > '9' {
			continue
		}
		// Check if this process's container root has the target library.
		// Skip PID 1 and kernel threads (no root fs).
		if pid == "1" || pid == "2" {
			continue
		}
		libPath := fmt.Sprintf("/host/proc/%s/root%s", pid, libRelPath)
		if _, err := os.Stat(libPath); err == nil {
			// Verify it's a container process by checking cgroup for kubepods
			cgPath := fmt.Sprintf("/host/proc/%s/cgroup", pid)
			cg, err := os.ReadFile(cgPath)
			if err != nil || !strings.Contains(string(cg), "kubepods") {
				continue
			}
			pidNum, _ := strconv.ParseUint(pid, 10, 32)
			return uint32(pidNum)
		}
	}
	return 0
}

func getBootTime() time.Time {
	data, err := os.ReadFile("/host/proc/stat")
	if err != nil {
		data, _ = os.ReadFile("/proc/stat")
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "btime ") {
			if epoch, err := strconv.ParseInt(strings.Fields(line)[1], 10, 64); err == nil {
				return time.Unix(epoch, 0)
			}
		}
	}
	return time.Now()
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
