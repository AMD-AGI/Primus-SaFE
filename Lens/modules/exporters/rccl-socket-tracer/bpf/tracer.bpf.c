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

// anpNetConnect(int dev, ncclNetCommConfig*, void* handle, void** sendComm, ...)
SEC("uprobe/anp_net_connect")
int handle_anp_connect(struct pt_regs *ctx)
{
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_ANP_CONNECT);
    e->retval = (int)PT_REGS_PARM1(ctx);  // dev id

    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 ts = bpf_ktime_get_ns();
    bpf_map_update_elem(&anp_connect_start, &pid_tgid, &ts, BPF_ANY);

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("uretprobe/anp_net_connect")
int handle_anp_connect_ret(struct pt_regs *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u64 *start = bpf_map_lookup_elem(&anp_connect_start, &pid_tgid);
    if (!start)
        return 0;

    __u64 duration = bpf_ktime_get_ns() - *start;
    bpf_map_delete_elem(&anp_connect_start, &pid_tgid);

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_ANP_CONNECT_RET);
    e->duration_ns = duration;
    e->retval = (int)PT_REGS_RC(ctx);  // ncclResult_t (0=success)

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
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    fill_event_base(e, EVT_ANP_ISEND);
    e->duration_ns = (long)PT_REGS_PARM3(ctx);  // size
    e->retval = (int)PT_REGS_PARM4(ctx);  // tag

    bpf_ringbuf_submit(e, 0);
    return 0;
}

// anpNetIrecv(void* recvComm, int nbufs, ...)
SEC("uprobe/anp_net_irecv")
int handle_anp_irecv(struct pt_regs *ctx)
{
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

char LICENSE[] SEC("license") = "GPL";
