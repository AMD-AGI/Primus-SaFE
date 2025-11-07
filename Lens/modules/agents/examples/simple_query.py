"""Simple Query Example for GPU Usage Analysis Agent."""

import os
from langchain_openai import ChatOpenAI

from gpu_usage_agent import GPUUsageAnalysisAgent


def main():
    """运行简单查询示例"""
    
    # 配置
    LENS_API_URL = os.getenv("LENS_API_URL", "http://localhost:8080")
    OPENAI_API_KEY = os.getenv("OPENAI_API_KEY")
    
    if not OPENAI_API_KEY:
        print("错误：请设置 OPENAI_API_KEY 环境变量")
        return
    
    # 初始化 LLM
    llm = ChatOpenAI(
        model="gpt-4",
        api_key=OPENAI_API_KEY,
        temperature=0
    )
    
    # 初始化 Agent
    agent = GPUUsageAnalysisAgent(
        llm=llm,
        api_base_url=LENS_API_URL,
        cluster_name=None  # 使用默认集群
    )
    
    # 示例查询
    queries = [
        "最近7天的GPU使用率趋势如何？",
        "当前集群有多少GPU在使用？",
        "ml-training namespace 的GPU使用情况",
    ]
    
    for query in queries:
        print(f"\n{'='*60}")
        print(f"查询: {query}")
        print(f"{'='*60}")
        
        result = agent.chat(query)
        
        print(f"\n答案:\n{result['answer']}")
        
        if result['insights']:
            print(f"\n洞察:")
            for insight in result['insights']:
                print(f"  - {insight}")
        
        if result.get('debug_info'):
            print(f"\n调试信息:")
            print(f"  意图: {result['debug_info'].get('intent')}")
            print(f"  迭代次数: {result['debug_info'].get('iterations')}")


if __name__ == "__main__":
    main()

