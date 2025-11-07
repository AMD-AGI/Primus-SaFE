"""Metadata Query Example - 演示元数据获取和交互式澄清."""

import os
from langchain_openai import ChatOpenAI

from gpu_usage_agent import GPUUsageAnalysisAgent


def main():
    """运行元数据查询示例"""
    
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
        cluster_name=None
    )
    
    print("=" * 70)
    print("GPU Usage Analysis Agent - 元数据交互示例")
    print("=" * 70)
    print()
    
    # 示例 1: 笼统的查询，Agent 会主动获取元数据并提供选项
    print("【示例 1】笼统查询 - Agent 会主动反问")
    print("-" * 70)
    
    query1 = "我想看看GPU使用情况"
    print(f"用户: {query1}")
    print()
    
    result1 = agent.chat(query1)
    print(f"Agent: {result1['answer']}")
    print()
    
    if result1.get('debug_info', {}).get('intent'):
        print(f"[调试] 识别的意图: {result1['debug_info']['intent']}")
    print()
    
    # 示例 2: 查询某个维度但没有指定值
    print("=" * 70)
    print("【示例 2】指定维度但缺少具体值")
    print("-" * 70)
    
    query2 = "查询某个namespace的GPU使用率趋势"
    print(f"用户: {query2}")
    print()
    
    result2 = agent.chat(query2)
    print(f"Agent: {result2['answer']}")
    print()
    
    # 示例 3: 明确的查询（不需要澄清）
    print("=" * 70)
    print("【示例 3】明确的查询")
    print("-" * 70)
    
    query3 = "查询集群最近7天的GPU使用率趋势"
    print(f"用户: {query3}")
    print()
    
    result3 = agent.chat(query3)
    print(f"Agent: {result3['answer']}")
    print()
    
    if result3.get('insights'):
        print("关键洞察:")
        for insight in result3['insights']:
            print(f"  - {insight}")
    print()
    
    # 示例 4: 查询可用的筛选维度
    print("=" * 70)
    print("【示例 4】查询可用的筛选维度")
    print("-" * 70)
    
    query4 = "可以按什么维度筛选GPU使用数据？"
    print(f"用户: {query4}")
    print()
    
    result4 = agent.chat(query4)
    print(f"Agent: {result4['answer']}")
    print()
    
    print("=" * 70)
    print("示例演示完成")
    print("=" * 70)


if __name__ == "__main__":
    main()

