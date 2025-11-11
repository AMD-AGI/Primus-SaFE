"""测试简化后的GPU Usage Agent"""

import logging
from langchain_openai import ChatOpenAI
from agent import GPUUsageAnalysisAgent

# 配置日志
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)

def test_agent():
    """测试简化后的agent"""
    
    # 初始化LLM（使用OpenAI或其他兼容的API）
    llm = ChatOpenAI(
        model="gpt-3.5-turbo",
        temperature=0
    )
    
    # 初始化Agent
    agent = GPUUsageAnalysisAgent(
        llm=llm,
        api_base_url="http://localhost:8080",  # 替换为实际的API地址
        cluster_name=None,  # 可选：指定集群名称
        cache_enabled=False  # 测试时不启用缓存
    )
    
    # 测试查询1：查询集群使用率
    print("\n" + "="*60)
    print("测试查询1：查询集群使用率")
    print("="*60)
    result = agent.chat("最近7天集群GPU使用率怎么样？")
    print(f"\n回答: {result['answer']}")
    print(f"\n数据维度: {list(result['data'].keys())}")
    
    # 测试查询2：查询所有维度
    print("\n" + "="*60)
    print("测试查询2：查询所有维度")
    print("="*60)
    result = agent.chat("分析一下最近7天GPU使用情况")
    print(f"\n回答: {result['answer']}")
    print(f"\n数据维度: {list(result['data'].keys())}")
    
    # 测试查询3：需要澄清
    print("\n" + "="*60)
    print("测试查询3：需要澄清")
    print("="*60)
    result = agent.chat("GPU使用情况")
    print(f"\n回答: {result['answer']}")
    print(f"\n需要澄清: {result['needs_clarification']}")
    
    # 测试查询4：查询特定namespace
    print("\n" + "="*60)
    print("测试查询4：查询特定namespace")
    print("="*60)
    result = agent.chat("查询ml-training这个namespace最近30天的使用情况")
    print(f"\n回答: {result['answer']}")
    if result['data'].get('requested_dimension'):
        print(f"\n请求的维度: {result['data']['requested_dimension']['dimension']}")
        print(f"维度值: {result['data']['requested_dimension']['dimension_value']}")
    
    print("\n" + "="*60)
    print("测试完成")
    print("="*60)


if __name__ == "__main__":
    test_agent()

