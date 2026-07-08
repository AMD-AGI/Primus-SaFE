#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 24);
} events SEC(".maps");

struct rdma_send_event {
	__u32 qpn;
	__u32 pid;
	__u32 opcode;
	__u32 pad;
	__u64 bytes;
};

/*
 * libibverbs struct ibv_qp: qp_num at offset 52 (after 6 pointers + u32 handle).
 * struct ibv_send_wr: sg_list @16, num_sge @24, opcode @28.
 * struct ibv_sge: length @8.
 */
#define OFF_IBV_QP_QP_NUM 52
#define OFF_WR_SG_LIST 16
#define OFF_WR_NUM_SGE 24
#define OFF_WR_OPCODE 28
#define OFF_SGE_LENGTH 8
#define MAX_SGE 8

SEC("uprobe/bnxt_re_post_send")
int trace_post_send(struct pt_regs *ctx)
{
	void *qp_ptr = (void *)PT_REGS_PARM1(ctx);
	void *wr_ptr = (void *)PT_REGS_PARM2(ctx);

	struct rdma_send_event ev = {};
	__u32 qp_num = 0;
	__u32 opcode = 0;
	void *sg_list = NULL;
	int num_sge = 0;
	__u64 total = 0;
	int i;
	char *sgep;

	if (bpf_probe_read_user(&qp_num, sizeof(qp_num), (char *)qp_ptr + OFF_IBV_QP_QP_NUM))
		return 0;
	if (bpf_probe_read_user(&opcode, sizeof(opcode), (char *)wr_ptr + OFF_WR_OPCODE))
		return 0;
	if (bpf_probe_read_user(&sg_list, sizeof(sg_list), (char *)wr_ptr + OFF_WR_SG_LIST))
		return 0;
	if (bpf_probe_read_user(&num_sge, sizeof(num_sge), (char *)wr_ptr + OFF_WR_NUM_SGE))
		return 0;

	if (num_sge < 0)
		num_sge = 0;
	if (num_sge > MAX_SGE)
		num_sge = MAX_SGE;

	sgep = (char *)sg_list;

#pragma unroll
	for (i = 0; i < MAX_SGE; i++) {
		__u32 len;

		if (i >= num_sge)
			break;
		if (!sgep)
			break;
		if (bpf_probe_read_user(&len, sizeof(len), sgep + OFF_SGE_LENGTH))
			break;
		total += len;
		sgep += 16;
	}

	ev.qpn = qp_num;
	ev.pid = bpf_get_current_pid_tgid() >> 32;
	ev.opcode = opcode;
	ev.bytes = total;

	bpf_ringbuf_output(&events, &ev, sizeof(ev), 0);
	return 0;
}

char LICENSE[] SEC("license") = "GPL";
