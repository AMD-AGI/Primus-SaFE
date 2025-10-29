import subprocess
import re


def get_rdma_links():
    """
    获取当前机器的 RDMA 设备信息
    返回格式：列表，每个元素是字典：
    {
        "link": "mlx5_1/1",
        "state": "ACTIVE",
        "physical_state": "LINK_UP",
        "netdev": "eth0"
    }
    """
    result = subprocess.run(["rdma", "link", "show"], capture_output=True, text=True)
    links = []
    for line in result.stdout.strip().split("\n"):
        # 示例: link mlx5_1/1 state ACTIVE physical_state LINK_UP netdev eth0
        m = re.match(r"link (\S+) state (\S+) physical_state (\S+) netdev (\S+)", line)
        if m:
            links.append({
                "link": m.group(1),
                "state": m.group(2),
                "physical_state": m.group(3),
                "netdev": m.group(4)
            })
    return links


def get_rdma_statistics():
    """
    获取 RDMA 设备的 counter 信息
    返回格式：字典，key 为 link 名称，value 为 counter 字典
    {
        "mlx5_8/1": {"rx_write_requests": -481413250, "rx_read_requests": 797596171, ...}
    }
    """
    result = subprocess.run(["rdma", "statistic", "show"], capture_output=True, text=True)
    stats = {}
    for line in result.stdout.strip().split("\n"):
        # 每行以 "link mlx5_8/1" 开头，后面是 key value 对
        parts = line.split()
        if len(parts) < 2 or parts[0] != "link":
            continue
        link_name = parts[1]
        counters = {}
        for i in range(2, len(parts), 2):
            key = parts[i]
            value = int(parts[i + 1])
            counters[key] = value
        stats[link_name] = counters
    return stats

