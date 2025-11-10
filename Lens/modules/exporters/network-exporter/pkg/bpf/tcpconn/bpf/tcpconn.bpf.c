#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"

struct event_t {
    __u32 pid;
    __u16 sport;
    __u16 dport;
    __u16 family;
    __u8 saddr[4];
    __u8 daddr[4];
    __u8 saddr_v6[16];
    __u8 daddr_v6[16];
    char typ[16];
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 24);
} events SEC(".maps");

SEC("kprobe/tcp_close")
int probe_tcp_close(struct pt_regs *ctx) {
    struct sock *sock_ptr = (struct sock *)(ctx->di);
    struct event_t event = {};

    // Get process ID
    event.pid = bpf_get_current_pid_tgid() >> 32;

    // Fill local port and remote port
    bpf_probe_read(&event.sport, sizeof(event.sport), &sock_ptr->__sk_common.skc_num);
    bpf_probe_read(&event.dport, sizeof(event.dport), &sock_ptr->__sk_common.skc_dport);

    // Convert to host byte order
    event.sport = __builtin_bswap16(event.sport);
    event.dport = __builtin_bswap16(event.dport);

    bpf_probe_read(&event.family, sizeof(event.family), &sock_ptr->__sk_common.skc_family);

    // Fill IPv4 addresses
    if (event.family == 2) {
        bpf_probe_read(&event.saddr, sizeof(event.saddr), &sock_ptr->__sk_common.skc_rcv_saddr);
        bpf_probe_read(&event.daddr, sizeof(event.daddr), &sock_ptr->__sk_common.skc_daddr);
    }

    // Fill IPv6 addresses
    if (event.family == 10) {
        bpf_probe_read(&event.saddr_v6, sizeof(event.saddr_v6), &sock_ptr->__sk_common.skc_v6_rcv_saddr);
        bpf_probe_read(&event.daddr_v6, sizeof(event.daddr_v6), &sock_ptr->__sk_common.skc_v6_daddr);
    }

    // Set type to "close"
    __builtin_memcpy(&event.typ, "close", sizeof("close") - 1);

    // Output event to ringbuf
    bpf_ringbuf_output(&events, &event, sizeof(event), 0);

    return 0;

}


SEC("kprobe/tcp_connect")
int probe_tcp_connect(struct pt_regs *ctx) {
    struct sock *sock_ptr = (struct sock *)(ctx->di);
    struct event_t event = {};

    // Get process ID
    event.pid = bpf_get_current_pid_tgid() >> 32;

    // Fill local port and remote port
    bpf_probe_read(&event.sport, sizeof(event.sport), &sock_ptr->__sk_common.skc_num);
    bpf_probe_read(&event.dport, sizeof(event.dport), &sock_ptr->__sk_common.skc_dport);

    // Convert to host byte order
    event.sport = __builtin_bswap16(event.sport);
    event.dport = __builtin_bswap16(event.dport);

    bpf_probe_read(&event.family, sizeof(event.family), &sock_ptr->__sk_common.skc_family);

    // Fill IPv4 addresses
    if (event.family == 2) {
        bpf_probe_read(&event.saddr, sizeof(event.saddr), &sock_ptr->__sk_common.skc_rcv_saddr);
        bpf_probe_read(&event.daddr, sizeof(event.daddr), &sock_ptr->__sk_common.skc_daddr);
    }

    // Fill IPv6 addresses
    if (event.family == 10) {
        bpf_probe_read(&event.saddr_v6, sizeof(event.saddr_v6), &sock_ptr->__sk_common.skc_v6_rcv_saddr);
        bpf_probe_read(&event.daddr_v6, sizeof(event.daddr_v6), &sock_ptr->__sk_common.skc_v6_daddr);
    }

    // Set type to "connect"
    __builtin_memcpy(&event.typ, "connect", sizeof("connect") - 1);

    // Output event to ringbuf
    bpf_ringbuf_output(&events, &event, sizeof(event), 0);

    return 0;

}

char LICENSE[] SEC("license") = "GPL";
