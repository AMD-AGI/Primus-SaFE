package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags "-O2 -g -Wall -D__TARGET_ARCH_x86" -target amd64 tracer ../bpf/tracer.bpf.c -- -I/usr/include -I../bpf

const (
	EvtConnectEnter     = 1
	EvtConnectExit      = 2
	EvtAcceptExit       = 3
	EvtTCPState         = 4
	EvtTCPRetransmit    = 5
	EvtTCPReset         = 6
	EvtSendmsgSlow      = 7
	EvtRecvmsgSlow      = 8
	EvtClose            = 9
	EvtAnpConnect       = 20
	EvtAnpConnectRet    = 21
	EvtAnpAccept        = 22
	EvtAnpAcceptRet     = 23
	EvtAnpIsend         = 24
	EvtAnpIrecv         = 25
	EvtAnpCloseSend     = 26
	EvtAnpCloseRecv     = 27
	EvtIbvCreateQP      = 30
	EvtIbvModifyQP      = 31
	EvtIbvModifyQPRet   = 32
	EvtIonicPollCqEnter = 40
	EvtIonicPollCqExit  = 41
	EvtIonicModifyQP    = 42
	EvtIonicModifyQPRet = 43
	EvtIonicCqeError    = 44
	EvtIonicCreateQP    = 45
	EvtIonicPostSend    = 46
	EvtIbAsyncEvent     = 50
	EvtIonicPortEvent   = 51
	EvtKfdEvictQueues   = 60
	EvtKfdRestoreQueues = 61
	EvtGpuJobTimedout   = 62
	EvtGpuReset         = 63
	EvtGpuXgmiRasErr    = 64
	EvtGpuPoison        = 65
	EvtSigsegv          = 70
	EvtHsaSignalOp      = 71
)

var eventNames = map[uint32]string{
	EvtConnectEnter:     "CONNECT",
	EvtConnectExit:      "CONNECT_DONE",
	EvtAcceptExit:       "ACCEPT_DONE",
	EvtTCPState:         "TCP_STATE",
	EvtTCPRetransmit:    "TCP_RETX",
	EvtTCPReset:         "TCP_RESET",
	EvtSendmsgSlow:      "SEND_SLOW",
	EvtRecvmsgSlow:      "RECV_SLOW",
	EvtClose:            "CLOSE",
	EvtAnpConnect:       "ANP_CONNECT",
	EvtAnpConnectRet:    "ANP_CONNECT_RET",
	EvtAnpAccept:        "ANP_ACCEPT",
	EvtAnpAcceptRet:     "ANP_ACCEPT_RET",
	EvtAnpIsend:         "ANP_ISEND",
	EvtAnpIrecv:         "ANP_IRECV",
	EvtAnpCloseSend:     "ANP_CLOSE_SEND",
	EvtAnpCloseRecv:     "ANP_CLOSE_RECV",
	EvtIbvCreateQP:      "IBV_CREATE_QP",
	EvtIbvModifyQP:      "IBV_MODIFY_QP",
	EvtIbvModifyQPRet:   "IBV_MODIFY_QP_RET",
	EvtIonicPollCqEnter: "IONIC_POLL_CQ",
	EvtIonicPollCqExit:  "IONIC_POLL_CQ_RET",
	EvtIonicModifyQP:    "IONIC_MODIFY_QP",
	EvtIonicModifyQPRet: "IONIC_MODIFY_QP_RET",
	EvtIonicCqeError:    "IONIC_CQE_ERROR",
	EvtIonicCreateQP:    "IONIC_CREATE_QP",
	EvtIonicPostSend:    "IONIC_POST_SEND",
	EvtIbAsyncEvent:     "IB_ASYNC_EVENT",
	EvtIonicPortEvent:   "IONIC_PORT_EVENT",
	EvtKfdEvictQueues:   "KFD_EVICT_QUEUES",
	EvtKfdRestoreQueues: "KFD_RESTORE_QUEUES",
	EvtGpuJobTimedout:   "GPU_JOB_TIMEDOUT",
	EvtGpuReset:         "GPU_RESET",
	EvtGpuXgmiRasErr:    "GPU_XGMI_RAS_ERR",
	EvtGpuPoison:        "GPU_POISON",
	EvtSigsegv:          "SIGSEGV",
	EvtHsaSignalOp:      "HSA_SIGNAL_OP",
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

type rcclStats struct {
	TxBytes   uint64
	TxOps     uint64
	RxOps     uint64
	PodName   string
	DeviceIdx uint32
	LastSeen  int64
}

var (
	outputDir      string
	containerCache sync.Map // host PID -> ContainerInfo
	fileCache      sync.Map // "container/pid" -> *os.File
	nsFilter       string
	prometheusAddr = getEnv("PROMETHEUS_ADDR", ":9190")
	rcclStatsMap   sync.Map // host PID (uint32) -> *rcclStats
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

	// SIGSEGV capture with fault address
	tryAttach("force_sig_fault", func() (link.Link, error) {
		return link.Kprobe("force_sig_fault", objs.HandleSigsegv, nil)
	})

	// Layer 4: ionic RDMA driver kprobes (kernel-wide, no library path needed)
	tryAttach("ionic_poll_cq", func() (link.Link, error) {
		return link.Kprobe("ionic_poll_cq", objs.HandleIonicPollCq, nil)
	})
	tryAttach("ionic_poll_cq_ret", func() (link.Link, error) {
		return link.Kretprobe("ionic_poll_cq", objs.HandleIonicPollCqRet, nil)
	})
	tryAttach("ionic_modify_qp", func() (link.Link, error) {
		return link.Kprobe("ionic_modify_qp", objs.HandleIonicModifyQp, nil)
	})
	tryAttach("ionic_modify_qp_ret", func() (link.Link, error) {
		return link.Kretprobe("ionic_modify_qp", objs.HandleIonicModifyQpRet, nil)
	})
	tryAttach("ionic_create_qp", func() (link.Link, error) {
		return link.Kprobe("ionic_create_qp", objs.HandleIonicCreateQp, nil)
	})
	tryAttach("ionic_post_send", func() (link.Link, error) {
		return link.Kprobe("ionic_post_send", objs.HandleIonicPostSend, nil)
	})

	// IB async events (system-wide, catches QP_FATAL, PORT_ERR etc.)
	tryAttach("ib_dispatch_event", func() (link.Link, error) {
		return link.Kprobe("ib_dispatch_event", objs.HandleIbDispatchEvent, nil)
	})
	tryAttach("ionic_port_event", func() (link.Link, error) {
		return link.Kprobe("ionic_port_event", objs.HandleIonicPortEvent, nil)
	})

	// Layer 5: GPU/amdgpu/KFD probes (best-effort, module may not be loaded)
	tryAttach("kfd_process_evict_queues", func() (link.Link, error) {
		return link.Kprobe("kfd_process_evict_queues", objs.HandleKfdEvictQueues, nil)
	})
	tryAttach("kfd_process_restore_queues", func() (link.Link, error) {
		return link.Kprobe("kfd_process_restore_queues", objs.HandleKfdRestoreQueues, nil)
	})
	tryAttach("amdgpu_job_timedout", func() (link.Link, error) {
		return link.Kprobe("amdgpu_job_timedout", objs.HandleGpuJobTimedout, nil)
	})
	tryAttach("amdgpu_device_gpu_recover", func() (link.Link, error) {
		return link.Kprobe("amdgpu_device_gpu_recover", objs.HandleGpuReset, nil)
	})
	tryAttach("amdgpu_xgmi_query_ras_error_count", func() (link.Link, error) {
		return link.Kprobe("amdgpu_xgmi_query_ras_error_count", objs.HandleXgmiRasErr, nil)
	})
	tryAttach("amdgpu_umc_pasid_poison_handler", func() (link.Link, error) {
		return link.Kprobe("amdgpu_umc_pasid_poison_handler", objs.HandleGpuPoison, nil)
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

			pid := findRDMATrainingPid(anpRelPath)
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
			// C++ mangled symbols for RCCL ANP (ROCm 7.1 / RCCL 2.27.x)
			attachU("anp_connect", hostAnpPath, "_Z13anpNetConnectiP23ncclNetCommConfig_v10_tPvPS1_PP24ncclNetDeviceHandle_v7_t", objs.HandleAnpConnect)
			attachUR("anp_connect_ret", hostAnpPath, "_Z13anpNetConnectiP23ncclNetCommConfig_v10_tPvPS1_PP24ncclNetDeviceHandle_v7_t", objs.HandleAnpConnectRet)
			attachU("anp_accept", hostAnpPath, "_Z12anpNetAcceptPvPS_PP24ncclNetDeviceHandle_v7_t", objs.HandleAnpAccept)
			attachUR("anp_accept_ret", hostAnpPath, "_Z12anpNetAcceptPvPS_PP24ncclNetDeviceHandle_v7_t", objs.HandleAnpAcceptRet)
			attachU("anp_isend", hostAnpPath, "_Z11anpNetIsendPvS_miS_S_PS_", objs.HandleAnpIsend)
			attachU("anp_irecv", hostAnpPath, "_Z11anpNetIrecvPviPS_PmPiS0_S0_S0_", objs.HandleAnpIrecv)
			attachU("anp_close_send", hostAnpPath, "_Z15anpNetCloseSendPv", objs.HandleAnpCloseSend)
			attachU("anp_close_recv", hostAnpPath, "_Z15anpNetCloseRecvPv", objs.HandleAnpCloseRecv)

			// libibverbs.so
			if _, err := os.Stat(hostIbvPath); err == nil {
				attachU("ibv_create_qp", hostIbvPath, "ibv_create_qp", objs.HandleIbvCreateQp)
				attachU("ibv_modify_qp", hostIbvPath, "ibv_modify_qp", objs.HandleIbvModifyQp)
				attachUR("ibv_modify_qp_ret", hostIbvPath, "ibv_modify_qp", objs.HandleIbvModifyQpRet)
			}

			// HSA signal tracking for crash context
			hsaLib := fmt.Sprintf("/host/proc/%d/root/opt/venv/lib/python3.10/site-packages/torch/lib/libhsa-runtime64.so", pid)
			if _, err := os.Stat(hsaLib); err == nil {
				attachU("hsa_signal_store_screlease", hsaLib, "hsa_signal_store_screlease", objs.HandleHsaSignalOp)
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

	// Start Prometheus metrics HTTP server for per-QP traffic stats
	go func() {
		http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
			nodeName := os.Getenv("NODE_NAME")

			// Global activity counters
			counterNames := []string{
				"rccl_tracer_connect_total",
				"rccl_tracer_post_send_total",
				"rccl_tracer_poll_cq_total",
				"rccl_tracer_modify_qp_total",
				"rccl_tracer_create_qp_total",
				"rccl_tracer_sigsegv_total",
				"rccl_tracer_kfd_evict_total",
				"rccl_tracer_connect_error_total",
				"rccl_tracer_cqe_error_total",
				"rccl_tracer_ib_async_event_total",
				"rccl_tracer_hsa_signal_ops_total",
			}

			for i, name := range counterNames {
				key := uint32(i)
				var val uint64
				if err := objs.ActivityCounters.Lookup(key, &val); err == nil {
					fmt.Fprintf(w, "%s{node=\"%s\"} %d\n", name, nodeName, val)
				}
			}

			// Per-device activity counters
			deviceNames := []string{"ionic_0", "ionic_2", "ionic_3", "ionic_4", "ionic_5", "ionic_7", "ionic_8", "ionic_9"}
			deviceIdxMap := []uint32{0, 2, 3, 4, 5, 7, 8, 9}
			for _, devIdx := range deviceIdxMap {
				for ci, cname := range counterNames {
					key := devIdx*16 + uint32(ci)
					var val uint64
					if err := objs.DeviceCounters.Lookup(key, &val); err == nil && val > 0 {
						devName := fmt.Sprintf("ionic_%d", devIdx)
						fmt.Fprintf(w, "%s{node=\"%s\",device=\"%s\"} %d\n", cname, nodeName, devName, val)
					}
				}
			}
			_ = deviceNames

			// Per-QP traffic (with device)
			var qpKey struct {
				QpNum     uint32
				DeviceIdx uint32
			}
			var qpVal struct {
				TxBytes uint64
				TxOps   uint64
				RxBytes uint64
				RxOps   uint64
			}

			iter := objs.QpTraffic.Iterate()
			for iter.Next(&qpKey, &qpVal) {
				devName := fmt.Sprintf("ionic_%d", qpKey.DeviceIdx)
				fmt.Fprintf(w, "rccl_tracer_qp_tx_ops{node=\"%s\",device=\"%s\",qp_num=\"%d\"} %d\n",
					nodeName, devName, qpKey.QpNum, qpVal.TxOps)
			}

			// Per-rank RCCL traffic with device label (scheme B: Go aggregation)
			now := time.Now().Unix()
			rcclStatsMap.Range(func(k, v interface{}) bool {
				pid := k.(uint32)
				stats := v.(*rcclStats)
				if now-atomic.LoadInt64(&stats.LastSeen) > 300 {
					rcclStatsMap.Delete(k)
					return true
				}
				containerPid := resolveContainerPid(pid)
				podName := stats.PodName
				devName := fmt.Sprintf("ionic_%d", stats.DeviceIdx)
				txBytes := atomic.LoadUint64(&stats.TxBytes)
				txOps := atomic.LoadUint64(&stats.TxOps)
				rxOps := atomic.LoadUint64(&stats.RxOps)
				fmt.Fprintf(w, "rccl_tracer_rccl_tx_bytes{node=\"%s\",pod=\"%s\",device=\"%s\",rank_pid=\"%d\"} %d\n",
					nodeName, podName, devName, containerPid, txBytes)
				fmt.Fprintf(w, "rccl_tracer_rccl_tx_ops{node=\"%s\",pod=\"%s\",device=\"%s\",rank_pid=\"%d\"} %d\n",
					nodeName, podName, devName, containerPid, txOps)
				fmt.Fprintf(w, "rccl_tracer_rccl_rx_ops{node=\"%s\",pod=\"%s\",device=\"%s\",rank_pid=\"%d\"} %d\n",
					nodeName, podName, devName, containerPid, rxOps)
				return true
			})

			fmt.Fprintf(w, "rccl_tracer_up{node=\"%s\"} 1\n", nodeName)
		})
		log.Printf("Prometheus metrics server listening on %s/metrics", prometheusAddr)
		if err := http.ListenAndServe(prometheusAddr, nil); err != nil {
			log.Printf("Warning: Prometheus server failed: %v", err)
		}
	}()

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

		// Aggregate RCCL traffic from ANP events (scheme B: Go-side aggregation)
		switch evt.EventType {
		case EvtAnpIsend:
			key := evt.Pid
			val, _ := rcclStatsMap.LoadOrStore(key, &rcclStats{PodName: cinfo.PodName, DeviceIdx: evt.OldState})
			stats := val.(*rcclStats)
			atomic.StoreInt64(&stats.LastSeen, time.Now().Unix())
			atomic.AddUint64(&stats.TxBytes, uint64(evt.DurationNs))
			atomic.AddUint64(&stats.TxOps, 1)
			if evt.OldState != 0 {
				stats.DeviceIdx = evt.OldState
			}
		case EvtAnpIrecv:
			key := evt.Pid
			val, _ := rcclStatsMap.LoadOrStore(key, &rcclStats{PodName: cinfo.PodName})
			stats := val.(*rcclStats)
			atomic.StoreInt64(&stats.LastSeen, time.Now().Unix())
			atomic.AddUint64(&stats.RxOps, 1)
		}
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
		return fmt.Sprintf("%s duration=%.1fms result=%s device=ionic_%d", base, durMs, ncclErr, e.OldState)
	case EvtAnpAccept:
		return base
	case EvtAnpAcceptRet:
		durMs := float64(e.DurationNs) / 1e6
		ncclErr := ncclResultName(e.Retval)
		return fmt.Sprintf("%s duration=%.1fms result=%s", base, durMs, ncclErr)
	case EvtAnpIsend:
		return fmt.Sprintf("%s size=%d tag=%d device=ionic_%d", base, e.DurationNs, e.Retval, e.OldState)
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
	case EvtIonicPollCqEnter:
		return base
	case EvtIonicPollCqExit:
		return fmt.Sprintf("%s completions=%d", base, e.Retval)
	case EvtIonicModifyQP:
		return base
	case EvtIonicModifyQPRet:
		durMs := float64(e.DurationNs) / 1e6
		if e.Retval != 0 {
			return fmt.Sprintf("%s duration=%.1fms FAILED ret=%d", base, durMs, e.Retval)
		}
		return fmt.Sprintf("%s duration=%.1fms OK", base, durMs)
	case EvtIonicCqeError:
		return fmt.Sprintf("%s qp_num=%d status=%d(%s) opcode=%d", base, e.OldState, e.Retval, ibWcStatusName(e.Retval), e.NewState)
	case EvtIonicCreateQP:
		return base
	case EvtIbAsyncEvent:
		evtType := ibEventTypeName(e.Retval)
		if e.OldState != 0 {
			return fmt.Sprintf("%s event=%s qp_num=%d", base, evtType, e.OldState)
		}
		if e.NewState != 0 {
			return fmt.Sprintf("%s event=%s port=%d", base, evtType, e.NewState)
		}
		return fmt.Sprintf("%s event=%s", base, evtType)
	case EvtIonicPortEvent:
		return base
	case EvtIonicPostSend:
		if e.OldState != 0 {
			return fmt.Sprintf("%s qp_num=%d device=ionic_%d", base, e.OldState, e.NewState)
		}
		return base
	case EvtKfdEvictQueues:
		return fmt.Sprintf("%s *** GPU QUEUE EVICTION", base)
	case EvtKfdRestoreQueues:
		return fmt.Sprintf("%s GPU queues restored", base)
	case EvtGpuJobTimedout:
		return fmt.Sprintf("%s *** GPU JOB TIMEOUT", base)
	case EvtGpuReset:
		return fmt.Sprintf("%s *** GPU RESET/RECOVERY", base)
	case EvtGpuXgmiRasErr:
		return fmt.Sprintf("%s *** XGMI RAS ERROR", base)
	case EvtGpuPoison:
		return fmt.Sprintf("%s *** GPU MEMORY POISON (ECC uncorrectable)", base)
	case EvtSigsegv:
		faultAddr := binary.LittleEndian.Uint64(e.Saddr[:8])
		lastSignal := binary.LittleEndian.Uint64(e.Daddr[:8])
		codeStr := "UNKNOWN"
		switch e.OldState {
		case 1:
			codeStr = "SEGV_MAPERR"
		case 2:
			codeStr = "SEGV_ACCERR"
		}
		s := fmt.Sprintf("%s *** SIGSEGV code=%s fault_addr=0x%x", base, codeStr, faultAddr)
		if lastSignal != 0 {
			s += fmt.Sprintf(" last_hsa_signal=0x%x", lastSignal)
		}
		return s
	case EvtHsaSignalOp:
		return base
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

func ibWcStatusName(s int32) string {
	switch s {
	case 0:
		return "SUCCESS"
	case 1:
		return "LOC_LEN_ERR"
	case 2:
		return "LOC_QP_OP_ERR"
	case 3:
		return "LOC_EEC_OP_ERR"
	case 4:
		return "LOC_PROT_ERR"
	case 5:
		return "WR_FLUSH_ERR"
	case 6:
		return "MW_BIND_ERR"
	case 7:
		return "BAD_RESP_ERR"
	case 8:
		return "LOC_ACCESS_ERR"
	case 9:
		return "REM_INV_REQ_ERR"
	case 10:
		return "REM_ACCESS_ERR"
	case 11:
		return "REM_OP_ERR"
	case 12:
		return "RETRY_EXC_ERR"
	case 13:
		return "RNR_RETRY_EXC_ERR"
	case 14:
		return "LOC_RDD_VIOL_ERR"
	case 15:
		return "REM_INV_RD_REQ_ERR"
	case 16:
		return "REM_ABORT_ERR"
	case 17:
		return "INV_EECN_ERR"
	case 18:
		return "INV_EEC_STATE_ERR"
	case 19:
		return "FATAL_ERR"
	case 20:
		return "RESP_TIMEOUT_ERR"
	case 21:
		return "GENERAL_ERR"
	default:
		return fmt.Sprintf("WC_ERR_%d", s)
	}
}

func ibEventTypeName(t int32) string {
	switch t {
	case 0:
		return "CQ_ERR"
	case 1:
		return "QP_FATAL"
	case 2:
		return "QP_REQ_ERR"
	case 3:
		return "QP_ACCESS_ERR"
	case 4:
		return "COMM_EST"
	case 5:
		return "SQ_DRAINED"
	case 6:
		return "PATH_MIG_ERR"
	case 7:
		return "PATH_MIG"
	case 8:
		return "PORT_ERR"
	case 9:
		return "PORT_ACTIVE"
	case 10:
		return "LID_CHANGE"
	case 11:
		return "PKEY_CHANGE"
	case 12:
		return "SM_CHANGE"
	case 13:
		return "CLIENT_REREGISTER"
	case 14:
		return "GID_CHANGE"
	default:
		return fmt.Sprintf("EVENT_%d", t)
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

func findRDMATrainingPid(libRelPath string) uint32 {
	entries, _ := os.ReadDir("/host/proc")
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid := entry.Name()
		if pid[0] < '1' || pid[0] > '9' {
			continue
		}

		ibPath := fmt.Sprintf("/host/proc/%s/root/dev/infiniband", pid)
		if _, err := os.Stat(ibPath); err != nil {
			continue
		}

		cgPath := fmt.Sprintf("/host/proc/%s/cgroup", pid)
		cg, err := os.ReadFile(cgPath)
		if err != nil || !strings.Contains(string(cg), "kubepods") {
			continue
		}

		libPath := fmt.Sprintf("/host/proc/%s/root%s", pid, libRelPath)
		if _, err := os.Stat(libPath); err == nil {
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
