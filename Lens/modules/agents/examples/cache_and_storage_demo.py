"""演示 LLM 缓存和 Chat History 存储功能"""

import requests
import time
from typing import Optional

# API 基础 URL
BASE_URL = "http://localhost:8001"


def chat(query: str, session_id: Optional[str] = None, cluster_name: str = "x-flannel"):
    """发送对话请求"""
    payload = {
        "query": query,
        "cluster_name": cluster_name,
    }
    
    if session_id:
        payload["session_id"] = session_id
    
    response = requests.post(
        f"{BASE_URL}/api/gpu-analysis/chat",
        json=payload
    )
    
    return response.json()


def get_cache_stats():
    """获取缓存统计"""
    response = requests.get(f"{BASE_URL}/api/gpu-analysis/cache/stats")
    return response.json()


def get_storage_stats():
    """获取存储统计"""
    response = requests.get(f"{BASE_URL}/api/gpu-analysis/storage/stats")
    return response.json()


def list_conversations(limit: int = 10):
    """列出对话记录"""
    response = requests.get(
        f"{BASE_URL}/api/gpu-analysis/storage/conversations",
        params={"limit": limit}
    )
    return response.json()


def search_conversations(query: str):
    """搜索对话记录"""
    response = requests.get(
        f"{BASE_URL}/api/gpu-analysis/storage/search",
        params={"query": query}
    )
    return response.json()


def demo_cache():
    """演示缓存功能"""
    print("\n" + "="*60)
    print("演示 1: LLM API 缓存")
    print("="*60)
    
    # 第一次查询（缓存未命中）
    print("\n1. 第一次查询（缓存未命中）...")
    start_time = time.time()
    result1 = chat("当前集群有多少GPU？")
    time1 = time.time() - start_time
    print(f"   耗时: {time1:.2f}s")
    print(f"   答案: {result1['answer'][:100]}...")
    
    # 查看缓存统计
    stats1 = get_cache_stats()
    print(f"\n   缓存统计:")
    if stats1.get("enabled"):
        print(f"   - 缓存后端: {stats1.get('backend')}")
        print(f"   - 缓存条目数: {stats1.get('stats', {}).get('size', 0)}")
    
    # 第二次查询相同的问题（缓存命中）
    print("\n2. 第二次查询相同问题（应该缓存命中）...")
    start_time = time.time()
    result2 = chat("当前集群有多少GPU？")
    time2 = time.time() - start_time
    print(f"   耗时: {time2:.2f}s")
    print(f"   提速: {(time1 - time2) / time1 * 100:.1f}%")
    
    # 查看更新后的缓存统计
    stats2 = get_cache_stats()
    if stats2.get("enabled"):
        print(f"\n   更新后的缓存统计:")
        print(f"   - 缓存条目数: {stats2.get('stats', {}).get('size', 0)}")
        print(f"   - 缓存命中次数: {stats2.get('stats', {}).get('total_hits', 0)}")


def demo_storage():
    """演示存储功能"""
    print("\n" + "="*60)
    print("演示 2: Chat History 存储")
    print("="*60)
    
    # 第一轮对话
    print("\n1. 第一轮对话...")
    result1 = chat("最近7天的GPU使用率趋势如何？")
    session_id = result1.get("debug_info", {}).get("session_id")
    print(f"   会话ID: {session_id}")
    print(f"   答案: {result1['answer'][:100]}...")
    
    # 第二轮对话（使用同一个 session_id，自动加载历史）
    print("\n2. 第二轮对话（使用相同 session_id）...")
    result2 = chat("那哪个 namespace 使用最多？", session_id=session_id)
    print(f"   答案: {result2['answer'][:100]}...")
    
    # 查看存储统计
    print("\n3. 查看存储统计...")
    stats = get_storage_stats()
    if stats.get("enabled"):
        print(f"   存储后端: {stats.get('backend')}")
        print(f"   总对话数: {stats.get('stats', {}).get('total_conversations', 0)}")
        print(f"   保留天数: {stats.get('retention_days')}")
    
    # 列出最近的对话
    print("\n4. 列出最近的对话...")
    conversations = list_conversations(limit=5)
    print(f"   找到 {conversations['count']} 个对话:")
    for conv in conversations['conversations'][:3]:
        print(f"   - {conv['session_id'][:8]}... (更新于 {conv['updated_at']})")
    
    # 搜索对话
    print("\n5. 搜索对话...")
    search_results = search_conversations("GPU使用率")
    print(f"   找到 {search_results['count']} 个匹配的对话")


def demo_multi_turn_conversation():
    """演示多轮对话"""
    print("\n" + "="*60)
    print("演示 3: 多轮对话（自动保存和加载历史）")
    print("="*60)
    
    questions = [
        "当前集群有多少GPU？",
        "使用率最高的是哪个 namespace？",
        "这个 namespace 最近的趋势如何？",
        "有什么优化建议吗？"
    ]
    
    session_id = None
    
    for i, question in enumerate(questions, 1):
        print(f"\n{i}. 用户: {question}")
        result = chat(question, session_id=session_id)
        
        # 第一次对话后获取 session_id
        if session_id is None:
            session_id = result.get("debug_info", {}).get("session_id")
            print(f"   [新会话 ID: {session_id}]")
        
        answer = result['answer']
        print(f"   Agent: {answer[:150]}{'...' if len(answer) > 150 else ''}")
        
        time.sleep(1)  # 稍作延迟，模拟真实对话


def demo_management():
    """演示管理功能"""
    print("\n" + "="*60)
    print("演示 4: 缓存和存储管理")
    print("="*60)
    
    # 查看缓存统计
    print("\n1. 缓存统计...")
    cache_stats = get_cache_stats()
    if cache_stats.get("enabled"):
        print(f"   后端: {cache_stats.get('backend')}")
        stats = cache_stats.get('stats', {})
        print(f"   条目数: {stats.get('size', 0)}")
        print(f"   命中次数: {stats.get('total_hits', 0)}")
        if 'total_size_bytes' in stats:
            size_mb = stats['total_size_bytes'] / 1024 / 1024
            print(f"   总大小: {size_mb:.2f} MB")
    
    # 查看存储统计
    print("\n2. 存储统计...")
    storage_stats = get_storage_stats()
    if storage_stats.get("enabled"):
        print(f"   后端: {storage_stats.get('backend')}")
        stats = storage_stats.get('stats', {})
        print(f"   对话数: {stats.get('total_conversations', 0)}")
        if 'total_size_bytes' in stats:
            size_mb = stats['total_size_bytes'] / 1024 / 1024
            print(f"   总大小: {size_mb:.2f} MB")
        print(f"   最早: {stats.get('oldest_conversation', 'N/A')}")
        print(f"   最新: {stats.get('newest_conversation', 'N/A')}")
    
    # 可选：清理操作（注释掉以避免误操作）
    # print("\n3. 清理缓存...")
    # response = requests.post(f"{BASE_URL}/api/gpu-analysis/cache/cleanup")
    # print(f"   清理结果: {response.json()}")


def main():
    """主函数"""
    print("="*60)
    print("LLM 缓存和 Chat History 存储功能演示")
    print("="*60)
    print("\n确保 Agent API 服务正在运行：")
    print("  cd Lens/modules/agents/api")
    print("  python main.py")
    print("\n开始演示...")
    
    try:
        # 演示 1: 缓存功能
        demo_cache()
        
        # 演示 2: 存储功能
        demo_storage()
        
        # 演示 3: 多轮对话
        demo_multi_turn_conversation()
        
        # 演示 4: 管理功能
        demo_management()
        
        print("\n" + "="*60)
        print("演示完成！")
        print("="*60)
        print("\n查看更多信息：")
        print("  - API 文档: http://localhost:8001/docs")
        print("  - 缓存统计: http://localhost:8001/api/gpu-analysis/cache/stats")
        print("  - 存储统计: http://localhost:8001/api/gpu-analysis/storage/stats")
        
    except requests.exceptions.ConnectionError:
        print("\n错误: 无法连接到 API 服务")
        print("请确保 Agent API 正在运行在 http://localhost:8001")
    except Exception as e:
        print(f"\n错误: {e}")
        import traceback
        traceback.print_exc()


if __name__ == "__main__":
    main()

