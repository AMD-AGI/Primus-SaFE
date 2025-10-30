import json
from statistics import mean

def analyze_flash_attn(data):
    results = {}
    flash_attn_tests = data["tests"]["flash_attn"]

    # Used to find slowest rank
    slowest_rank = None
    max_latency_sum = -1

    # Aggregate data from all dimensions
    for test in flash_attn_tests:
        rank = test["rank"]
        hostname = test["hostname"]

        # Collect tflops
        for key, val in test["tflops"].items():
            if key not in results:
                results[key] = {
                    "fwd_tflops": [],
                    "bwd_tflops": [],
                    "fwd_time": [],
                    "bwd_time": [],
                }
            results[key]["fwd_tflops"].append(val["fwd_tflops"])
            results[key]["bwd_tflops"].append(val["bwd_tflops"])

        # Collect latency
        total_latency = 0
        total_fwd_tflops = 0
        total_bwd_tflops = 0
        total_count = 0
        avg_fwd_tflops = 0
        avg_bwd_tflops = 0
        for key, val in test["latency_us"].items():
            results[key]["fwd_time"].append(val["fwd_time"])
            results[key]["bwd_time"].append(val["bwd_time"])
            total_latency += val["fwd_time"] + val["bwd_time"]

        for key, val in test["tflops"].items():
            results[key]["fwd_tflops"].append(val["fwd_tflops"])
            total_fwd_tflops += val["fwd_tflops"]
            results[key]["bwd_tflops"].append(val["bwd_tflops"])
            total_bwd_tflops += val["bwd_tflops"]
            total_count += 1

        avg_fwd_tflops = total_fwd_tflops / total_count
        avg_bwd_tflops = total_bwd_tflops / total_count

        # Determine slowest rank
        if total_latency > max_latency_sum:
            max_latency_sum = total_latency
            slowest_rank = {
                "rank": rank,
                "hostname": hostname,
                "latency_sum": total_latency,
                "fwd_tflops": avg_fwd_tflops,
                "bwd_tflops": avg_bwd_tflops,
            }

    # Calculate average values
    avg_results = {}
    for key, val in results.items():
        avg_results[key] = {
            "avg_fwd_tflops": mean(val["fwd_tflops"]),
            "avg_bwd_tflops": mean(val["bwd_tflops"]),
            "avg_fwd_time": mean(val["fwd_time"]),
            "avg_bwd_time": mean(val["bwd_time"]),
        }

    return {
        "average": avg_results,
        "slowest_rank": slowest_rank,
    }


# Example
if __name__ == "__main__":
    with open("flash_attn.json") as f:
        data = json.load(f)

    result = analyze_flash_attn(data)
    with open("flash-attn-result.json", "w") as f:
        json.dump(result, f, indent=2)