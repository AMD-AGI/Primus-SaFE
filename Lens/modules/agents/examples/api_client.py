"""API Client Example for GPU Usage Analysis Agent."""

import requests
import json


def main():
    """API 客户端示例"""
    
    # API 基础 URL
    API_URL = "http://localhost:8001"
    
    # 1. 健康检查
    print("=" * 60)
    print("健康检查")
    print("=" * 60)
    
    response = requests.get(f"{API_URL}/health")
    print(f"状态: {response.status_code}")
    print(json.dumps(response.json(), indent=2, ensure_ascii=False))
    
    # 2. 获取能力列表
    print("\n" + "=" * 60)
    print("Agent 能力")
    print("=" * 60)
    
    response = requests.get(f"{API_URL}/api/gpu-analysis/capabilities")
    capabilities = response.json()
    print(json.dumps(capabilities, indent=2, ensure_ascii=False))
    
    # 3. 对话查询
    print("\n" + "=" * 60)
    print("对话查询")
    print("=" * 60)
    
    queries = [
        "最近7天的GPU使用率趋势如何？",
        "当前集群有多少GPU在使用？",
    ]
    
    for query in queries:
        print(f"\n查询: {query}")
        
        response = requests.post(
            f"{API_URL}/api/gpu-analysis/chat",
            json={
                "query": query,
                "cluster_name": None
            }
        )
        
        if response.status_code == 200:
            result = response.json()
            print(f"\n答案:\n{result['answer']}")
            
            if result.get('insights'):
                print(f"\n洞察:")
                for insight in result['insights']:
                    print(f"  - {insight}")
        else:
            print(f"错误: {response.status_code}")
            print(response.text)
        
        print("-" * 60)


if __name__ == "__main__":
    main()

