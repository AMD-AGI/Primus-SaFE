#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 24);
} events SEC(".maps");

struct rdma_ctrl_event {
	__u32 qpn;
	__u32 remote_qpn;
	__u8 remote_gid[16];
	__u16 remote_lid;
	__u8 port_num;
	__u8 pad;
	__u32 pid;
	__u32 attr_mask;
};

/* Offsets match include/rdma/ib_verbs.h struct ib_qp / ib_qp_attr (x86_64, Linux 5.15+). */
#define IB_QP_DEST_QPN 0x20000
#define OFF_IB_QP_QP_NUM 176
#define OFF_IB_QP_ATTR_DEST_QP_NUM 28
#define OFF_IB_QP_ATTR_AH_GRH_DGID 72
#define OFF_IB_QP_ATTR_AH_PORT_NUM 100
#define OFF_IB_QP_ATTR_AH_IB_DLID 112

SEC("kprobe/ib_modify_qp")
int trace_ib_modify_qp(struct pt_regs *ctx)
{
	int attr_mask = (int)PT_REGS_PARM3(ctx);

	if (!(attr_mask & IB_QP_DEST_QPN))
		return 0;

	void *qp = (void *)PT_REGS_PARM1(ctx);
	void *attr = (void *)PT_REGS_PARM2(ctx);

	struct rdma_ctrl_event ev = {};

	if (bpf_probe_read_kernel(&ev.qpn, sizeof(ev.qpn), (char *)qp + OFF_IB_QP_QP_NUM))
		return 0;
	if (bpf_probe_read_kernel(&ev.remote_qpn, sizeof(ev.remote_qpn),
				  (char *)attr + OFF_IB_QP_ATTR_DEST_QP_NUM))
		return 0;
	if (bpf_probe_read_kernel(ev.remote_gid, sizeof(ev.remote_gid),
				  (char *)attr + OFF_IB_QP_ATTR_AH_GRH_DGID))
		return 0;
	if (bpf_probe_read_kernel(&ev.remote_lid, sizeof(ev.remote_lid),
				  (char *)attr + OFF_IB_QP_ATTR_AH_IB_DLID))
		return 0;
	{
		__u32 port;

		if (bpf_probe_read_kernel(&port, sizeof(port), (char *)attr + OFF_IB_QP_ATTR_AH_PORT_NUM))
			return 0;
		ev.port_num = (__u8)port;
	}
	ev.pid = bpf_get_current_pid_tgid() >> 32;
	ev.attr_mask = (__u32)attr_mask;

	bpf_ringbuf_output(&events, &ev, sizeof(ev), 0);
	return 0;
}

char LICENSE[] SEC("license") = "GPL";
