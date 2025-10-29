import subprocess
import json

def get_rocm_smi_info():
    """
    调用 rocm-smi 获取 GPU 信息，并解析为 Python dict
    """
    try:
        result = subprocess.run(
            ["rocm-smi", "-t", "-f", "-P", "-u", "--showmemuse", "-b", "--json"],
            capture_output=True,
            text=True,
            check=True
        )
        data = json.loads(result.stdout)
        return data
    except subprocess.CalledProcessError as e:
        print("执行 rocm-smi 失败:", e.stderr)
        return {}
    except json.JSONDecodeError as e:
        print("解析 JSON 失败:", e)
        return {}

if __name__ == "__main__":
    gpu_info = get_rocm_smi_info()
    if gpu_info:
        for card, metrics in gpu_info.items():
            print(f"\n{card}:")
            for k, v in metrics.items():
                print(f"  {k}: {v}")
