"""GPU使用率分析Agent使用示例

这个示例展示了改造后的Agent的主要功能：
1. 集群趋势分析（带折线图）
2. Namespace分析
3. 用户占用分析（带表格）
4. 灵活的澄清机制
"""

import json
import logging
from langchain_openai import ChatOpenAI

from gpu_usage_agent.agent import GPUUsageAnalysisAgent

# 配置日志
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


def print_section(title: str):
    """打印分节标题"""
    print("\n" + "=" * 80)
    print(f"  {title}")
    print("=" * 80 + "\n")


def print_result(result: dict):
    """美化打印分析结果"""
    print("\n【回答】")
    print(result.get("answer", "无回答"))
    
    if result.get("needs_clarification"):
        print("\n⚠️ 需要澄清")
        return
    
    data = result.get("data", {})
    
    # 打印集群趋势数据
    if "cluster_trend" in data:
        cluster = data["cluster_trend"]
        if "chart_data" in cluster:
            chart = cluster["chart_data"]
            print(f"\n📊 【折线图数据】: {chart['title']}")
            print(f"   - 时间点数量: {len(chart['x_axis'])}")
            print(f"   - 序列数量: {len(chart['series'])}")
            for series in chart['series']:
                print(f"   - {series['name']}: {len(series['data'])} 个数据点")
    
    # 打印用户分析表格
    if "user_analysis" in data:
        user_analysis = data["user_analysis"]
        if "table_data" in user_analysis:
            table = user_analysis["table_data"]
            print(f"\n📋 【用户分析表格】")
            print(f"   - 列: {table.get('columns', [])}")
            print(f"   - 行数: {len(table.get('rows', []))}")
            
            # 打印前5行
            rows = table.get('rows', [])[:5]
            if rows:
                print("\n   前5个用户:")
                for i, row in enumerate(rows):
                    print(f"   {i+1}. 用户:{row[0]}, GPU占用:{row[1]}, 使用率:{row[2]}%, 问题评分:{row[4]}")
    
    # 打印Namespace分析
    if "namespace_analysis" in data:
        ns_analysis = data["namespace_analysis"]
        if "namespaces" in ns_analysis:
            namespaces = ns_analysis["namespaces"][:5]
            print(f"\n📦 【Namespace分析】")
            print(f"   前5个namespace:")
            for i, ns in enumerate(namespaces):
                print(f"   {i+1}. {ns['namespace']}: 使用率{ns['avg_utilization']}%, GPU占用{ns['avg_gpu_count']}")


def example_1_cluster_trend():
    """示例1: 查询集群趋势（折线图）"""
    print_section("示例1: 集群趋势分析")
    
    # 初始化Agent
    llm = ChatOpenAI(model="gpt-4", temperature=0)
    agent = GPUUsageAnalysisAgent(
        llm=llm,
        api_base_url="http://localhost:8080"
    )
    
    # 用户查询
    query = "最近7天集群GPU使用率和占用率的趋势是什么？给我一个折线图"
    print(f"用户: {query}")
    
    # 调用Agent
    result = agent.chat(query)
    print_result(result)


def example_2_user_analysis():
    """示例2: 用户占用分析（表格）"""
    print_section("示例2: 用户占用分析")
    
    # 初始化Agent
    llm = ChatOpenAI(model="gpt-4", temperature=0)
    agent = GPUUsageAnalysisAgent(
        llm=llm,
        api_base_url="http://localhost:8080"
    )
    
    # 用户查询
    query = "分析一下哪些用户占用了很多GPU但使用率很低，用表格展示"
    print(f"用户: {query}")
    
    # 调用Agent
    result = agent.chat(query)
    print_result(result)


def example_3_namespace_analysis():
    """示例3: Namespace分析"""
    print_section("示例3: Namespace分析")
    
    # 初始化Agent
    llm = ChatOpenAI(model="gpt-4", temperature=0)
    agent = GPUUsageAnalysisAgent(
        llm=llm,
        api_base_url="http://localhost:8080"
    )
    
    # 用户查询
    query = "最近30天各个namespace的GPU使用情况"
    print(f"用户: {query}")
    
    # 调用Agent
    result = agent.chat(query)
    print_result(result)


def example_4_specific_user():
    """示例4: 特定用户分析"""
    print_section("示例4: 特定用户分析")
    
    # 初始化Agent
    llm = ChatOpenAI(model="gpt-4", temperature=0)
    agent = GPUUsageAnalysisAgent(
        llm=llm,
        api_base_url="http://localhost:8080"
    )
    
    # 用户查询
    query = "zhangsan用户的GPU占用情况怎么样？"
    print(f"用户: {query}")
    
    # 调用Agent
    result = agent.chat(query)
    print_result(result)


def example_5_clarification():
    """示例5: 需要澄清的查询"""
    print_section("示例5: 需要澄清的查询")
    
    # 初始化Agent
    llm = ChatOpenAI(model="gpt-4", temperature=0)
    agent = GPUUsageAnalysisAgent(
        llm=llm,
        api_base_url="http://localhost:8080"
    )
    
    # 用户查询（不明确）
    query = "GPU"
    print(f"用户: {query}")
    
    # 调用Agent
    result = agent.chat(query)
    print_result(result)


def example_6_full_analysis():
    """示例6: 完整分析"""
    print_section("示例6: 完整分析")
    
    # 初始化Agent
    llm = ChatOpenAI(model="gpt-4", temperature=0)
    agent = GPUUsageAnalysisAgent(
        llm=llm,
        api_base_url="http://localhost:8080"
    )
    
    # 用户查询
    query = "分析一下最近的GPU使用情况"
    print(f"用户: {query}")
    
    # 调用Agent
    result = agent.chat(query)
    print_result(result)


def main():
    """运行所有示例"""
    print("\n" + "=" * 80)
    print("  GPU使用率分析Agent - 改造后功能演示")
    print("=" * 80)
    
    # 运行各个示例
    examples = [
        ("集群趋势分析", example_1_cluster_trend),
        ("用户占用分析", example_2_user_analysis),
        ("Namespace分析", example_3_namespace_analysis),
        ("特定用户分析", example_4_specific_user),
        ("需要澄清", example_5_clarification),
        ("完整分析", example_6_full_analysis)
    ]
    
    print("\n可用示例:")
    for i, (name, _) in enumerate(examples):
        print(f"{i+1}. {name}")
    
    choice = input("\n请选择要运行的示例 (1-6，或 'all' 运行所有): ").strip()
    
    if choice.lower() == 'all':
        for name, func in examples:
            try:
                func()
            except Exception as e:
                logger.error(f"运行示例 '{name}' 失败: {str(e)}")
    else:
        try:
            idx = int(choice) - 1
            if 0 <= idx < len(examples):
                examples[idx][1]()
            else:
                print("无效的选择")
        except ValueError:
            print("无效的输入")


if __name__ == "__main__":
    # 注意: 需要设置环境变量 OPENAI_API_KEY
    # 或使用其他LLM provider
    main()

