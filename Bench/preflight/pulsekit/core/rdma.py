import subprocess
import re


def get_rdma_links():
    """
    Get RDMA device information for the current machine
    Return format: list, each element is a dict:
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
        # Example: link mlx5_1/1 state ACTIVE physical_state LINK_UP netdev eth0
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
    Get RDMA device counter information
    Return format: dict, key is link name, value is counter dict
    {
        "mlx5_8/1": {"rx_write_requests": -481413250, "rx_read_requests": 797596171, ...}
    }
    """
    result = subprocess.run(["rdma", "statistic", "show"], capture_output=True, text=True)
    stats = {}
    for line in result.stdout.strip().split("\n"):
        # Each line starts with "link mlx5_8/1", followed by key value pairs
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


if __name__ == "__main__":
    print("RDMA Links:")
    for link in get_rdma_links():
        print(link)

    print("\nRDMA Statistics:")
    stats = get_rdma_statistics()
    for link, counters in stats.items():
        print(f"{link}: {counters}")
