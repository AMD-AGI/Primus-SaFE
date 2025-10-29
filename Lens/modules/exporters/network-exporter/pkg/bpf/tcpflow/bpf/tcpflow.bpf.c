#include "vmlinux.h"
#include "bpf_helpers.h"

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 24);
} events SEC(".maps");

struct tcp_probe {
    // Common fields for tracepoint
    unsigned short common_type;          // offset: 0, size: 2
    unsigned char common_flags;          // offset: 2, size: 1
    unsigned char common_preempt_count;  // offset: 3, size: 1
    int common_pid;                      // offset: 4, size: 4

    // Source and destination addresses
    __u8 saddr[sizeof(struct sockaddr_in6)]; // offset: 8, size: 28
    __u8 daddr[sizeof(struct sockaddr_in6)]; // offset: 36, size: 28

    // Port and protocol information
    __u16 sport;                         // offset: 64, size: 2
    __u16 dport;                         // offset: 66, size: 2
    __u16 family;                        // offset: 68, size: 2

    // Additional metadata
    __u32 mark;                          // offset: 72, size: 4
    __u16 data_len;                      // offset: 76, size: 2

    // TCP state information
    __u32 snd_nxt;                       // offset: 80, size: 4
    __u32 snd_una;                       // offset: 84, size: 4
    __u32 snd_cwnd;                      // offset: 88, size: 4
    __u32 ssthresh;                      // offset: 92, size: 4
    __u32 snd_wnd;                       // offset: 96, size: 4
    __u32 srtt;                          // offset: 100, size: 4
    __u32 rcv_wnd;                       // offset: 104, size: 4

    // Unique socket identifier
    __u64 sock_cookie;                   // offset: 112, size: 8
};

struct tcp_probe_event {
    __u8 saddr[sizeof(struct sockaddr_in6)];
    __u8 daddr[sizeof(struct sockaddr_in6)];
    // Port and protocol information
    __u16 sport;
    __u16 dport;
    __u16 family;
    __u16 reason;
    __u32 data_len;
    __u32 srtt;
    __u32 pid;
};


SEC("tracepoint/tcp/tcp_probe")
int trace_tcp_probe(struct tcp_probe *ctx) {
    if (ctx->data_len == 0) {
        return 0;
    }
    struct tcp_probe_event event = {};

    // Copy the source and destination addresses
    bpf_probe_read(&event.saddr, sizeof(event.saddr), &ctx->saddr);
    bpf_probe_read(&event.daddr, sizeof(event.daddr), &ctx->daddr);
    // Copy the port and protocol information
    event.sport = ctx->sport;
    event.dport = ctx->dport;
    event.family = ctx->family;
    event.data_len = ctx->data_len;
    event.srtt = ctx->srtt;
    event.pid = bpf_get_current_pid_tgid() >> 32;
    bpf_ringbuf_output(&events, &event, sizeof(event), 0);
    return 0;
}

char _license[] SEC("license") = "GPL";
