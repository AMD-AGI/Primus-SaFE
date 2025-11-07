"""GPU Usage Agent 简化版本 - 使用示例"""

import json
import logging
from langchain_openai import ChatOpenAI
from agent import GPUUsageAnalysisAgent

# 配置日志
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


def print_result(title: str, result: dict):
    """打印查询结果"""
    print("\n" + "="*70)
    print(f"  {title}")
    print("="*70)
    
    print(f"\n【摘要】\n{result['answer']}")
    
    if result.get('needs_clarification'):
        print("\n⚠️ 需要更多信息")
        return
    
    data = result.get('data', {})
    print(f"\n【数据维度】")
    for key in data.keys():
        if isinstance(data[key], list):
            print(f"  - {key}: {len(data[key])} 项")
        elif isinstance(data[key], dict):
            if 'statistics' in data[key]:
                stats = data[key]['statistics']
                print(f"  - {key}: avg={stats.get('average', 0):.1f}%, "
                      f"trend={stats.get('trend', 'unknown')}")
            else:
                print(f"  - {key}: {len(data[key])} 项")
    
    # 打印部分详细数据
    if 'cluster' in data and 'statistics' in data['cluster']:
        stats = data['cluster']['statistics']
        print(f"\n【集群统计】")
        print(f"  平均使用率: {stats.get('average', 0):.2f}%")
        print(f"  最大使用率: {stats.get('max', 0):.2f}%")
        print(f"  最小使用率: {stats.get('min', 0):.2f}%")
        print(f"  趋势: {stats.get('trend', 'unknown')}")
        print(f"  数据点数: {stats.get('sample_count', 0)}")


def main():
    """主函数"""
    print("\n" + "="*70)
    print("  GPU Usage Agent - 简化版本示例")
    print("="*70)
    
    # 初始化LLM
    # 注意：需要设置OPENAI_API_KEY环境变量
    try:
        llm = ChatOpenAI(
            model="gpt-3.5-turbo",
            temperature=0
        )
    except Exception as e:
        print(f"\n❌ 初始化LLM失败: {e}")
        print("请确保设置了OPENAI_API_KEY环境变量")
        return
    
    # 初始化Agent
    try:
        agent = GPUUsageAnalysisAgent(
            llm=llm,
            api_base_url="http://localhost:8080",  # 修改为实际的Lens API地址
            cluster_name=None,
            cache_enabled=False
        )
        print("\n✓ Agent初始化成功")
    except Exception as e:
        print(f"\n❌ 初始化Agent失败: {e}")
        print("请确保Lens API服务正在运行")
        return
    
    # 测试查询列表
    queries = [
        "最近7天集群GPU使用率怎么样？",
        "分析一下最近7天GPU使用情况",
        "查询ml-training这个namespace最近30天的使用情况",
        "GPU使用情况"  # 这个应该会要求澄清
    ]
    
    # 执行查询
    for query in queries:
        try:
            result = agent.chat(query)
            print_result(query, result)
        except Exception as e:
            print(f"\n❌ 查询失败: {query}")
            print(f"   错误: {e}")
            logger.exception("查询异常")
    
    print("\n" + "="*70)
    print("  示例完成")
    print("="*70 + "\n")


if __name__ == "__main__":
    main()

