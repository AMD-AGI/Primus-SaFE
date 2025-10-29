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

    // 获取进程 ID
    event.pid = bpf_get_current_pid_tgid() >> 32;

    // 填充本地端口和远程端口
    bpf_probe_read(&event.sport, sizeof(event.sport), &sock_ptr->__sk_common.skc_num);
    bpf_probe_read(&event.dport, sizeof(event.dport), &sock_ptr->__sk_common.skc_dport);

    // 转换到主机字节序
    event.sport = __builtin_bswap16(event.sport);
    event.dport = __builtin_bswap16(event.dport);

    bpf_probe_read(&event.family, sizeof(event.family), &sock_ptr->__sk_common.skc_family);

    // 填充 IPv4 地址
    if (event.family == 2) {
        bpf_probe_read(&event.saddr, sizeof(event.saddr), &sock_ptr->__sk_common.skc_rcv_saddr);
        bpf_probe_read(&event.daddr, sizeof(event.daddr), &sock_ptr->__sk_common.skc_daddr);
    }

    // 填充 IPv6 地址
    if (event.family == 10) {
        bpf_probe_read(&event.saddr_v6, sizeof(event.saddr_v6), &sock_ptr->__sk_common.skc_v6_rcv_saddr);
        bpf_probe_read(&event.daddr_v6, sizeof(event.daddr_v6), &sock_ptr->__sk_common.skc_v6_daddr);
    }

    // 设置类型为 "close"
    __builtin_memcpy(&event.typ, "close", sizeof("close") - 1);

    // 将事件输出到 ringbuf
    bpf_ringbuf_output(&events, &event, sizeof(event), 0);

    return 0;

}


SEC("kprobe/tcp_connect")
int probe_tcp_connect(struct pt_regs *ctx) {
    struct sock *sock_ptr = (struct sock *)(ctx->di);
    struct event_t event = {};

    // 获取进程 ID
    event.pid = bpf_get_current_pid_tgid() >> 32;

    // 填充本地端口和远程端口
    bpf_probe_read(&event.sport, sizeof(event.sport), &sock_ptr->__sk_common.skc_num);
    bpf_probe_read(&event.dport, sizeof(event.dport), &sock_ptr->__sk_common.skc_dport);

    // 转换到主机字节序
    event.sport = __builtin_bswap16(event.sport);
    event.dport = __builtin_bswap16(event.dport);

    bpf_probe_read(&event.family, sizeof(event.family), &sock_ptr->__sk_common.skc_family);

    // 填充 IPv4 地址
    if (event.family == 2) {
        bpf_probe_read(&event.saddr, sizeof(event.saddr), &sock_ptr->__sk_common.skc_rcv_saddr);
        bpf_probe_read(&event.daddr, sizeof(event.daddr), &sock_ptr->__sk_common.skc_daddr);
    }

    // 填充 IPv6 地址
    if (event.family == 10) {
        bpf_probe_read(&event.saddr_v6, sizeof(event.saddr_v6), &sock_ptr->__sk_common.skc_v6_rcv_saddr);
        bpf_probe_read(&event.daddr_v6, sizeof(event.daddr_v6), &sock_ptr->__sk_common.skc_v6_daddr);
    }

    // 设置类型为 "close"
    __builtin_memcpy(&event.typ, "connect", sizeof("connect") - 1);

    // 将事件输出到 ringbuf
    bpf_ringbuf_output(&events, &event, sizeof(event), 0);

    return 0;

}

char LICENSE[] SEC("license") = "GPL";
