// SPDX-License-Identifier: GPL-2.0
// rccl-socket-tracer: eBPF probes for diagnosing RCCL/NCCL bootstrap failures
//
// Layer 1: TCP connection lifecycle (connect, accept, state changes, resets)
// Layer 2: Socket syscall latency (connect/accept/send/recv timing)

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_endian.h>

#define AF_INET 2
#define AF_INET6 10
#define TASK_COMM_LEN 16
#define MAX_ENTRIES 65536

/*
 * Tracepoint context structs: use vmlinux.h definitions if available (CO-RE),
 * otherwise fall back to manual definitions. The vmlinux.h generated from
 * kernel BTF on newer kernels includes these; older ones may not.
 */

enum event_type {
    EVT_CONNECT_ENTER  = 1,
    EVT_CONNECT_EXIT   = 2,
    EVT_ACCEPT_EXIT    = 3,
    EVT_TCP_STATE      = 4,
    EVT_TCP_RETRANSMIT = 5,
    EVT_TCP_RESET      = 6,
    EVT_SENDMSG_SLOW   = 7,
    EVT_RECVMSG_SLOW   = 8,
    // Layer 3: RCCL/RDMA uprobe events
    EVT_ANP_CONNECT    = 20,
    EVT_ANP_CONNECT_RET= 21,
    EVT_ANP_ACCEPT     = 22,
    EVT_ANP_ACCEPT_RET = 23,
    EVT_ANP_ISEND      = 24,
    EVT_ANP_IRECV      = 25,
    EVT_ANP_CLOSE_SEND = 26,
    EVT_ANP_CLOSE_RECV = 27,
    EVT_IBV_CREATE_QP  = 30,
    EVT_IBV_MODIFY_QP  = 31,
    EVT_IBV_MODIFY_QP_RET = 32,
    // Layer 4: ionic RDMA driver kprobes
    EVT_IONIC_POLL_CQ_ENTER = 40,
    EVT_IONIC_POLL_CQ_EXIT  = 41,
    EVT_IONIC_MODIFY_QP     = 42,
    EVT_IONIC_MODIFY_QP_RET = 43,
    EVT_IONIC_CQE_ERROR     = 44,
    EVT_IONIC_CREATE_QP     = 45,
    EVT_IONIC_POST_SEND     = 46,
    EVT_IB_ASYNC_EVENT      = 50,
    EVT_IONIC_PORT_EVENT    = 51,
    // Layer 5: GPU/amdgpu/KFD probes
    EVT_KFD_EVICT_QUEUES  = 60,
    EVT_KFD_RESTORE_QUEUES= 61,
    EVT_GPU_JOB_TIMEDOUT  = 62,
    EVT_GPU_RESET          = 63,
    EVT_GPU_XGMI_RAS_ERR  = 64,
    EVT_GPU_POISON         = 65,
    EVT_SIGSEGV            = 70,
    EVT_HSA_SIGNAL_OP      = 71,
};

struct event {
    __u64 ts_ns;
    __u32 pid;
    __u32 tid;
    __u32 event_type;
    __u32 af;
    __u32 sport;
    __u32 dport;
    __u8  saddr[16];
    __u8  daddr[16];
    __s64 duration_ns;
    __s32 retval;
    __u32 old_state;
    __u32 new_state;
    char  comm[TASK_COMM_LEN];
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 24);
} events SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_ENTRIES);
    __type(key, __u64);
    __type(value, __u64);
} connect_start SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_ENTRIES);
    __type(key, __u64);
    __type(value, __u64);
} accept_start SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_ENTRIES);
    __type(key, __u64);
    __type(value, __u64);
} io_start SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u64);
} tracer_config SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, __u64);
    __type(value, __u8);
} cgroup_filter SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_ENTRIES);
    __type(key, __u32);
    __type(value, __u64);
} hsa_last_signal SEC(".maps");

struct qp_stats_key {
    __u32 qp_num;
    __u32 device_idx;
};

struct qp_stats_val {
    __u64 tx_bytes;
    __u64 tx_ops;
    __u64 rx_bytes;
    __u64 rx_ops;
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 16384);
    __type(key, struct qp_stats_key);
    __type(value, struct qp_stats_val);
} qp_traffic SEC(".maps");

// Global activity counters for Prometheus export
// Key: counter_id (0=connect_total, 1=post_send_total, 2=poll_cq_total,
//      3=modify_qp_total, 4=create_qp_total, 5=sigsegv_total,
//      6=kfd_evict_total, 7=connect_error_total, 8=cqe_error_total,
//      9=ib_async_total, 10=hsa_signal_total)
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 16);
    __type(key, __u32);
    __type(value, __u64);
} activity_counters SEC(".maps");

// Per-device activity counters: key = device_idx * 16 + counter_id
// Supports up to 10 devices × 16 counter types = 160 entries
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 160);
    __type(key, __u32);
    __type(value, __u64);
} device_counters SEC(".maps");

static __always_inline void bump_counter(__u32 idx)
{
    __u64 *val = bpf_map_lookup_elem(&activity_counters, &idx);
    if (val)
        __sync_fetch_and_add(val, 1);
}

static __always_inline void bump_device_counter(__u32 device_idx, __u32 counter_id)
{
    __u32 key = device_idx * 16 + counter_id;
    if (key >= 160) return;
    __u64 *val = bpf_map_lookup_elem(&device_counters, &key);
    if (val)
        __sync_fetch_and_add(val, 1);
}

// Read ionic device index from ib_qp pointer
// ib_qp->device (offset 0) → ib_device->name (offset varies, try common offsets)
static __always_inline __u32 read_device_idx_from_qp(void *qp)
{
    void *device = NULL;
    bpf_probe_read_kernel(&device, sizeof(device), qp); // ib_qp->device at offset 0
    if (!device) return 0;

    // Read device name - try offset 24 (common for ib_device.name in 6.x kernels)
    // name is char[IB_DEVICE_NAME_MAX] = char[64]
    char name[8] = {};
    bpf_probe_read_kernel(&name, sizeof(name), device + 24);

    // Parse "ionic_N" → N
    if (name[0] == 'i' && name[1] == 'o' && name[2] == 'n' &&
        name[3] == 'i' && name[4] == 'c' && name[5] == '_') {
        return name[6] - '0';  // single digit 0-9
    }
    return 0;
}

static __always_inline bool should_trace(void)
{
    __u64 cgid = bpf_get_current_cgroup_id();
    __u8 *val = bpf_map_lookup_elem(&cgroup_filter, &cgid);
    return val != NULL || bpf_map_lookup_elem(&cgroup_filter, &(__u64){0}) != NULL;
}

static __always_inline __u64 get_slow_threshold(void)
{
    __u32 key = 0;
    __u64 *val = bpf_map_lookup_elem(&tracer_config, &key);
    return val ? *val : 100000000ULL;
}

static __always_inline void fill_event_base(struct event *e, __u32 type)
{
    e->ts_ns = bpf_ktime_get_ns();
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    e->pid = pid_tgid >> 32;
    e->tid = (__u32)pid_tgid;
    e->event_type = type;
    bpf_get_current_comm(e->comm, sizeof(e->comm));
}

static __always_inline void read_sockaddr_in(struct event *e,
    const struct sockaddr *addr, bool is_dst)
{
    struct sockaddr_in sa = {};
    bpf_probe_read_user(&sa, sizeof(sa), addr);
    e->af = AF_INET;
    if (is_dst) {
        e->dport = bpf_ntohs(sa.sin_port);
        __builtin_memcpy(e->daddr, &sa.sin_addr, 4);
    } else {
        e->sport = bpf_ntohs(sa.sin_port);
        __builtin_memcpy(e->saddr, &sa.sin_addr, 4);
    }
}

// ===== Layer 1: TCP lifecycle =====
// inet_sock_set_state tracepoint removed: struct layout varies across kernels
// and causes BPF verifier failures. TCP state changes are still observable
// via connect() return codes and tcp_reset kprobe.

SEC("kprobe/tcp_reset")
int handle_tcp_reset(struct pt_regs *ctx)
{
    if (!should_trace())
        return 0;

    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_TCP_RESET);
    e->af = BPF_CORE_READ(sk, __sk_common.skc_family);
    e->sport = BPF_CORE_READ(sk, __sk_common.skc_num);
    e->dport = bpf_ntohs(BPF_CORE_READ(sk, __sk_common.skc_dport));

    if (e->af == AF_INET) {
        __u32 src = BPF_CORE_READ(sk, __sk_common.skc_rcv_saddr);
        __u32 dst = BPF_CORE_READ(sk, __sk_common.skc_daddr);
        __builtin_memcpy(e->saddr, &src, 4);
        __builtin_memcpy(e->daddr, &dst, 4);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

/*
 * tcp_retransmit_skb tracepoint removed: trace_event_raw_tcp_retransmit_skb
 * is not in all kernel BTF exports. TCP retransmissions are still observable
 * via inet_sock_set_state (repeated SYN_SENT) and kprobe/tcp_reset.
 */

// ===== Layer 2: Syscall latency =====

SEC("tracepoint/syscalls/sys_enter_connect")
int handle_connect_enter(struct trace_event_raw_sys_enter *ctx)
{
    if (!should_trace())
        return 0;

    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 ts = bpf_ktime_get_ns();
    bpf_map_update_elem(&connect_start, &pid_tgid, &ts, BPF_ANY);

    struct sockaddr *addr = (struct sockaddr *)ctx->args[1];
    __u16 family = 0;
    bpf_probe_read_user(&family, sizeof(family), &addr->sa_family);
    if (family != AF_INET)
        return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_CONNECT_ENTER);
    bump_counter(0);
    read_sockaddr_in(e, addr, true);

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_connect")
int handle_connect_exit(struct trace_event_raw_sys_exit *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 *start = bpf_map_lookup_elem(&connect_start, &pid_tgid);
    if (!start)
        return 0;

    __u64 duration = bpf_ktime_get_ns() - *start;
    bpf_map_delete_elem(&connect_start, &pid_tgid);

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_CONNECT_EXIT);
    e->duration_ns = duration;
    e->retval = ctx->ret;
    if (ctx->ret < 0)
        bump_counter(7);

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_accept4")
int handle_accept_enter(struct trace_event_raw_sys_enter *ctx)
{
    if (!should_trace())
        return 0;

    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 ts = bpf_ktime_get_ns();
    bpf_map_update_elem(&accept_start, &pid_tgid, &ts, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_accept4")
int handle_accept_exit(struct trace_event_raw_sys_exit *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 *start = bpf_map_lookup_elem(&accept_start, &pid_tgid);
    if (!start)
        return 0;

    __u64 duration = bpf_ktime_get_ns() - *start;
    bpf_map_delete_elem(&accept_start, &pid_tgid);

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_ACCEPT_EXIT);
    e->duration_ns = duration;
    e->retval = ctx->ret;

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_sendto")
int handle_sendto_enter(struct trace_event_raw_sys_enter *ctx)
{
    if (!should_trace())
        return 0;
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 ts = bpf_ktime_get_ns();
    bpf_map_update_elem(&io_start, &pid_tgid, &ts, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_sendto")
int handle_sendto_exit(struct trace_event_raw_sys_exit *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 *start = bpf_map_lookup_elem(&io_start, &pid_tgid);
    if (!start)
        return 0;

    __u64 duration = bpf_ktime_get_ns() - *start;
    bpf_map_delete_elem(&io_start, &pid_tgid);

    if (duration < get_slow_threshold())
        return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_SENDMSG_SLOW);
    e->duration_ns = duration;
    e->retval = ctx->ret;

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_recvfrom")
int handle_recvfrom_enter(struct trace_event_raw_sys_enter *ctx)
{
    if (!should_trace())
        return 0;
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 ts = bpf_ktime_get_ns();
    bpf_map_update_elem(&io_start, &pid_tgid, &ts, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_recvfrom")
int handle_recvfrom_exit(struct trace_event_raw_sys_exit *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 *start = bpf_map_lookup_elem(&io_start, &pid_tgid);
    if (!start)
        return 0;

    __u64 duration = bpf_ktime_get_ns() - *start;
    bpf_map_delete_elem(&io_start, &pid_tgid);

    if (duration < get_slow_threshold())
        return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_RECVMSG_SLOW);
    e->duration_ns = duration;
    e->retval = ctx->ret;

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ===== Layer 3: RCCL ANP (AINIC) + ibverbs uprobes =====
// These are auto-attached by userspace using the library paths found at runtime.
// SEC names use uprobe/ prefix; userspace resolves the actual binary path.

// Track anpNetConnect/anpNetAccept enter timestamps
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_ENTRIES);
    __type(key, __u64);
    __type(value, __u64);
} anp_connect_start SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_ENTRIES);
    __type(key, __u64);
    __type(value, __u64);
} ibv_modify_qp_start SEC(".maps");

// Per-sendComm connection info: local device + peer IP
struct comm_info {
    __u32 dev_idx;      // local ionic device index
    __u32 peer_ip;      // peer IPv4 address (network byte order)
};

// Map sendComm pointer → connection info (populated by anpNetConnect return)
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 8192);
    __type(key, __u64);              // sendComm pointer value
    __type(value, struct comm_info); // device + peer
} comm_to_dev SEC(".maps");

// Save connect enter args per thread
struct connect_args {
    __u64 start_ns;
    __u64 sendcomm_out_ptr;  // void** sendComm (arg4)
    __u32 dev_idx;           // int dev (arg1)
    __u32 peer_ip;           // extracted from handle->connectAddr
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_ENTRIES);
    __type(key, __u64);   // pid_tgid
    __type(value, struct connect_args);
} anp_connect_args SEC(".maps");

// anpNetConnect(int dev, ncclNetCommConfig*, void* handle, void** sendComm, ...)
// handle is ncclIbHandle* where connectAddr (sockaddr) is at offset 0
SEC("uprobe/anp_net_connect")
int handle_anp_connect(struct pt_regs *ctx)
{
    __u32 dev = (__u32)PT_REGS_PARM1(ctx);
    void *handle = (void *)PT_REGS_PARM3(ctx);

    // Read peer IP from handle->connectAddr (struct sockaddr_in at offset 0)
    // sockaddr_in layout: sa_family(2B) + sin_port(2B) + sin_addr(4B)
    __u32 peer_ip = 0;
    if (handle) {
        bpf_probe_read_user(&peer_ip, sizeof(peer_ip), handle + 4);  // sin_addr at offset 4
    }

    __u64 pid_tgid = bpf_get_current_pid_tgid();
    struct connect_args args = {
        .start_ns = bpf_ktime_get_ns(),
        .sendcomm_out_ptr = (__u64)PT_REGS_PARM4(ctx),
        .dev_idx = dev,
        .peer_ip = peer_ip,
    };
    bpf_map_update_elem(&anp_connect_args, &pid_tgid, &args, BPF_ANY);

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_ANP_CONNECT);
    e->retval = dev;
    // Store peer IP in daddr field
    __builtin_memcpy(e->daddr, &peer_ip, 4);
    e->af = AF_INET;

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("uretprobe/anp_net_connect")
int handle_anp_connect_ret(struct pt_regs *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    struct connect_args *args = bpf_map_lookup_elem(&anp_connect_args, &pid_tgid);
    if (!args)
        return 0;

    __u64 duration = bpf_ktime_get_ns() - args->start_ns;
    __u32 dev_idx = args->dev_idx;
    __u32 peer_ip = args->peer_ip;
    __u64 sendcomm_out = args->sendcomm_out_ptr;
    bpf_map_delete_elem(&anp_connect_args, &pid_tgid);

    int ret = (int)PT_REGS_RC(ctx);

    // On success, map sendComm → {device, peer_ip}
    if (ret == 0 && sendcomm_out != 0) {
        __u64 comm_ptr = 0;
        bpf_probe_read_user(&comm_ptr, sizeof(comm_ptr), (void *)sendcomm_out);
        if (comm_ptr != 0) {
            struct comm_info info = { .dev_idx = dev_idx, .peer_ip = peer_ip };
            bpf_map_update_elem(&comm_to_dev, &comm_ptr, &info, BPF_ANY);
        }
    }

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_ANP_CONNECT_RET);
    e->duration_ns = duration;
    e->retval = ret;
    e->old_state = dev_idx;
    __builtin_memcpy(e->daddr, &peer_ip, 4);
    e->af = AF_INET;

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// anpNetAccept(void* listenComm, void** recvComm, ...)
SEC("uprobe/anp_net_accept")
int handle_anp_accept(struct pt_regs *ctx)
{
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_ANP_ACCEPT);

    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 ts = bpf_ktime_get_ns();
    bpf_map_update_elem(&anp_connect_start, &pid_tgid, &ts, BPF_ANY);

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("uretprobe/anp_net_accept")
int handle_anp_accept_ret(struct pt_regs *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 *start = bpf_map_lookup_elem(&anp_connect_start, &pid_tgid);
    if (!start)
        return 0;

    __u64 duration = bpf_ktime_get_ns() - *start;
    bpf_map_delete_elem(&anp_connect_start, &pid_tgid);

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_ANP_ACCEPT_RET);
    e->duration_ns = duration;
    e->retval = (int)PT_REGS_RC(ctx);

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// anpNetIsend(void* sendComm, void* data, unsigned long size, int tag, ...)
SEC("uprobe/anp_net_isend")
int handle_anp_isend(struct pt_regs *ctx)
{
    bump_counter(11);

    __u64 comm_ptr = (__u64)PT_REGS_PARM1(ctx);
    __u32 dev_idx = 0;
    __u32 peer_ip = 0;
    struct comm_info *info = bpf_map_lookup_elem(&comm_to_dev, &comm_ptr);
    if (info) {
        dev_idx = info->dev_idx;
        peer_ip = info->peer_ip;
    }

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_ANP_ISEND);
    e->duration_ns = (long)PT_REGS_PARM3(ctx);  // size
    e->retval = (int)PT_REGS_PARM4(ctx);  // tag
    e->old_state = dev_idx;  // local device index
    __builtin_memcpy(e->daddr, &peer_ip, 4);  // peer IP
    e->af = AF_INET;

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// anpNetIrecv(void* recvComm, int nbufs, ...)
SEC("uprobe/anp_net_irecv")
int handle_anp_irecv(struct pt_regs *ctx)
{
    bump_counter(12);

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_ANP_IRECV);
    e->retval = (int)PT_REGS_PARM2(ctx);  // nbufs

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("uprobe/anp_net_close_send")
int handle_anp_close_send(struct pt_regs *ctx)
{
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_event_base(e, EVT_ANP_CLOSE_SEND);
    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("uprobe/anp_net_close_recv")
int handle_anp_close_recv(struct pt_regs *ctx)
{
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_event_base(e, EVT_ANP_CLOSE_RECV);
    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ibv_modify_qp - track QP state transitions and failures
SEC("uprobe/ibv_modify_qp")
int handle_ibv_modify_qp(struct pt_regs *ctx)
{
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_IBV_MODIFY_QP);

    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 ts = bpf_ktime_get_ns();
    bpf_map_update_elem(&ibv_modify_qp_start, &pid_tgid, &ts, BPF_ANY);

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("uretprobe/ibv_modify_qp")
int handle_ibv_modify_qp_ret(struct pt_regs *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 *start = bpf_map_lookup_elem(&ibv_modify_qp_start, &pid_tgid);
    if (!start)
        return 0;

    __u64 duration = bpf_ktime_get_ns() - *start;
    bpf_map_delete_elem(&ibv_modify_qp_start, &pid_tgid);

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_IBV_MODIFY_QP_RET);
    e->duration_ns = duration;
    e->retval = (int)PT_REGS_RC(ctx);  // 0=success, -1=error

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ===== Layer 4: ionic RDMA driver kprobes =====

struct poll_cq_args {
    __u64 wc_addr;  // void *wc cast to u64 (bpf2go can't handle pointer types)
    __u32 num_entries;
    __u32 _pad;
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_ENTRIES);
    __type(key, __u64);
    __type(value, struct poll_cq_args);
} poll_cq_enter SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_ENTRIES);
    __type(key, __u64);
    __type(value, __u64);
} ionic_modify_qp_start SEC(".maps");

// ibv_create_qp
SEC("uprobe/ibv_create_qp")
int handle_ibv_create_qp(struct pt_regs *ctx)
{
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_event_base(e, EVT_IBV_CREATE_QP);
    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ===== Layer 4 probes: ionic RDMA driver =====

SEC("kprobe/ionic_poll_cq")
int handle_ionic_poll_cq(struct pt_regs *ctx)
{
    if (!should_trace())
        return 0;

    __u64 pid_tgid = bpf_get_current_pid_tgid();

    struct poll_cq_args args = {};
    args.wc_addr = (__u64)PT_REGS_PARM3(ctx);
    args.num_entries = (__u32)PT_REGS_PARM2(ctx);
    bpf_map_update_elem(&poll_cq_enter, &pid_tgid, &args, BPF_ANY);

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_IONIC_POLL_CQ_ENTER);
    bump_counter(2);
    e->retval = args.num_entries;

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("kretprobe/ionic_poll_cq")
int handle_ionic_poll_cq_ret(struct pt_regs *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    struct poll_cq_args *args = bpf_map_lookup_elem(&poll_cq_enter, &pid_tgid);
    if (!args)
        return 0;

    __u64 wc_addr = args->wc_addr;
    bpf_map_delete_elem(&poll_cq_enter, &pid_tgid);

    int ret = (int)PT_REGS_RC(ctx);

    #pragma unroll
    for (int i = 0; i < 8; i++) {
        if (i >= ret)
            break;

        __u32 status = 0;
        __u32 qp_num = 0;
        __u32 opcode = 0;

        void *wc_entry = (void *)(wc_addr + i * 48);
        bpf_probe_read_kernel(&status, sizeof(status), wc_entry);
        bpf_probe_read_kernel(&opcode, sizeof(opcode), wc_entry + 4);
        bpf_probe_read_kernel(&qp_num, sizeof(qp_num), wc_entry + 16);

        if (status != 0) {
            bump_counter(8);
            struct event *err = bpf_ringbuf_reserve(&events, sizeof(*err), 0);
            if (err) {
                fill_event_base(err, EVT_IONIC_CQE_ERROR);
                err->retval = status;
                err->old_state = qp_num;
                err->new_state = opcode;
                bpf_ringbuf_submit(err, 0);
            }
        }
    }

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_IONIC_POLL_CQ_EXIT);
    e->retval = ret;

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("kprobe/ionic_modify_qp")
int handle_ionic_modify_qp(struct pt_regs *ctx)
{
    if (!should_trace())
        return 0;

    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 ts = bpf_ktime_get_ns();
    bpf_map_update_elem(&ionic_modify_qp_start, &pid_tgid, &ts, BPF_ANY);

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_IONIC_MODIFY_QP);
    bump_counter(3);

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("kretprobe/ionic_modify_qp")
int handle_ionic_modify_qp_ret(struct pt_regs *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 *start = bpf_map_lookup_elem(&ionic_modify_qp_start, &pid_tgid);
    if (!start)
        return 0;

    __u64 duration = bpf_ktime_get_ns() - *start;
    bpf_map_delete_elem(&ionic_modify_qp_start, &pid_tgid);

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_IONIC_MODIFY_QP_RET);
    e->duration_ns = duration;
    e->retval = (int)PT_REGS_RC(ctx);

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("kprobe/ionic_create_qp")
int handle_ionic_create_qp(struct pt_regs *ctx)
{
    if (!should_trace())
        return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_IONIC_CREATE_QP);
    bump_counter(4);

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("kprobe/ionic_post_send")
int handle_ionic_post_send(struct pt_regs *ctx)
{
    // No cgroup filter - RDMA data plane runs in kernel context
    void *qp = (void *)PT_REGS_PARM1(ctx);
    __u32 qp_num = 0;
    bpf_probe_read_kernel(&qp_num, sizeof(qp_num), qp + 168);

    __u32 dev_idx = read_device_idx_from_qp(qp);

    bump_counter(1);
    bump_device_counter(dev_idx, 1);  // per-device post_send

    struct qp_stats_key key = { .qp_num = qp_num, .device_idx = dev_idx };
    struct qp_stats_val *val = bpf_map_lookup_elem(&qp_traffic, &key);
    if (val) {
        __sync_fetch_and_add(&val->tx_ops, 1);
    } else {
        struct qp_stats_val new_val = { .tx_ops = 1 };
        bpf_map_update_elem(&qp_traffic, &key, &new_val, BPF_NOEXIST);
    }

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_event_base(e, EVT_IONIC_POST_SEND);
    e->old_state = qp_num;
    e->new_state = dev_idx;
    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("kprobe/ib_dispatch_event")
int handle_ib_dispatch_event(struct pt_regs *ctx)
{
    void *ib_event = (void *)PT_REGS_PARM1(ctx);

    __u32 event_type = 0;
    bpf_probe_read_kernel(&event_type, sizeof(event_type), ib_event + 8);

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_IB_ASYNC_EVENT);
    bump_counter(9);
    e->retval = event_type;

    if (event_type >= 1 && event_type <= 5) {
        void *qp_ptr = NULL;
        bpf_probe_read_kernel(&qp_ptr, sizeof(qp_ptr), ib_event + 16);
        if (qp_ptr) {
            __u32 qp_num = 0;
            bpf_probe_read_kernel(&qp_num, sizeof(qp_num), qp_ptr + 168);
            e->old_state = qp_num;
        }
    }
    if (event_type >= 8 && event_type <= 9) {
        __u32 port_num = 0;
        bpf_probe_read_kernel(&port_num, sizeof(port_num), ib_event + 16);
        e->new_state = port_num;
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("kprobe/ionic_port_event")
int handle_ionic_port_event(struct pt_regs *ctx)
{
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_event_base(e, EVT_IONIC_PORT_EVENT);
    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ===== Layer 5: GPU/amdgpu/KFD probes =====

// kfd_process_evict_queues - fires when GPU queues are evicted (the "queue evicted" dmesg message)
SEC("kprobe/kfd_process_evict_queues")
int handle_kfd_evict_queues(struct pt_regs *ctx)
{
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_event_base(e, EVT_KFD_EVICT_QUEUES);
    bump_counter(6);
    bpf_ringbuf_submit(e, 0);
    return 0;
}

// kfd_process_restore_queues - fires when evicted queues are restored
SEC("kprobe/kfd_process_restore_queues")
int handle_kfd_restore_queues(struct pt_regs *ctx)
{
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_event_base(e, EVT_KFD_RESTORE_QUEUES);
    bpf_ringbuf_submit(e, 0);
    return 0;
}

// amdgpu_job_timedout - fires when a GPU job exceeds the scheduler timeout
SEC("kprobe/amdgpu_job_timedout")
int handle_gpu_job_timedout(struct pt_regs *ctx)
{
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_event_base(e, EVT_GPU_JOB_TIMEDOUT);
    bpf_ringbuf_submit(e, 0);
    return 0;
}

// amdgpu_device_gpu_recover - fires when GPU reset/recovery is triggered
SEC("kprobe/amdgpu_device_gpu_recover")
int handle_gpu_reset(struct pt_regs *ctx)
{
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_event_base(e, EVT_GPU_RESET);
    bpf_ringbuf_submit(e, 0);
    return 0;
}

// amdgpu_xgmi_query_ras_error_count - XGMI link RAS error query
SEC("kprobe/amdgpu_xgmi_query_ras_error_count")
int handle_xgmi_ras_err(struct pt_regs *ctx)
{
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_event_base(e, EVT_GPU_XGMI_RAS_ERR);
    bpf_ringbuf_submit(e, 0);
    return 0;
}

// amdgpu_umc_pasid_poison_handler - GPU memory poison (ECC uncorrectable)
SEC("kprobe/amdgpu_umc_pasid_poison_handler")
int handle_gpu_poison(struct pt_regs *ctx)
{
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;
    fill_event_base(e, EVT_GPU_POISON);
    bpf_ringbuf_submit(e, 0);
    return 0;
}

// ===== Layer 6: SIGSEGV crash context =====

SEC("kprobe/force_sig_fault")
int handle_sigsegv(struct pt_regs *ctx)
{
    int sig = (int)PT_REGS_PARM1(ctx);
    if (sig != 11)
        return 0;

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_SIGSEGV);
    bump_counter(5);
    e->retval = sig;

    __u64 fault_addr = (__u64)PT_REGS_PARM3(ctx);
    __builtin_memcpy(e->saddr, &fault_addr, 8);

    e->old_state = (__u32)PT_REGS_PARM2(ctx);

    __u32 pid = e->pid;
    __u64 *last_sig = bpf_map_lookup_elem(&hsa_last_signal, &pid);
    if (last_sig) {
        __builtin_memcpy(e->daddr, last_sig, 8);
    }

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("uprobe/hsa_signal_store_screlease")
int handle_hsa_signal_op(struct pt_regs *ctx)
{
    bump_counter(10);
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 pid = pid_tgid >> 32;
    __u64 signal_handle = (__u64)PT_REGS_PARM1(ctx);

    bpf_map_update_elem(&hsa_last_signal, &pid, &signal_handle, BPF_ANY);
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
