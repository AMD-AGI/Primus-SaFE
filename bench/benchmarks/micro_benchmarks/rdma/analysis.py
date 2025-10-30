import json
import statistics

from dask.array import delete


def analyze(data):
    results = {}

    tests = data["tests"]["bandwidth"]["alltoall"]
    for comm_type in ["allreduce", "alltoall"]:
        comm_results = {}
        for group_size in ["2", "4", "128"]:
            if group_size not in tests[comm_type]:
                continue
            groups = tests[comm_type][group_size]

            avg_bandwidths = []
            group_bandwidths = []

            for g in groups:
                avg_bw = statistics.mean(g["bandwidth"])
                avg_bandwidths.append(avg_bw)
                group_bandwidths.append({
                    "nodes": g["nodes"],
                    "avg_bandwidth": avg_bw
                })

            comm_results[group_size] = {
                "avg_bandwidth": statistics.mean(avg_bandwidths),
                "groups": group_bandwidths
            }

        # Find slowest combination among 2 and 4 nodes respectively
        slowest_groups = {}
        for size in ["2", "4"]:
            if size not in comm_results:
                continue
            slowest_group = min(
                comm_results[size]["groups"],
                key=lambda g: g["avg_bandwidth"]
            )
            slowest_groups[size] = slowest_group

        comm_results["slowest_groups"] = slowest_groups
        results[comm_type] = comm_results

    return results


# Example usage
if __name__ == "__main__":
    with open("pulsekit-perf-test-rsrm8.json") as f:
        data = json.load(f)

    analysis = analyze(data)
    # Output to result.json
    with open("result.json", "w") as f:
        json.dump(analysis, f, indent=2)