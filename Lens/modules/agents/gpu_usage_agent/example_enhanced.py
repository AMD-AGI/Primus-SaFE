"""GPU Usage Agent 增强版本 - 使用示例"""

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


def print_cluster_analysis(cluster_data: dict):
    """打印cluster分析结果"""
    print("\n" + "="*70)
    print("  【Cluster级别分析】")
    print("="*70)
    
    if "error" in cluster_data:
        print(f"❌ 错误: {cluster_data['error']}")
        return
    
    if "summary" in cluster_data:
        print(f"\n摘要: {cluster_data['summary']}")
    
    if "statistics" in cluster_data:
        stats = cluster_data["statistics"]
        print(f"\n统计信息:")
        print(f"  平均使用率: {stats.get('average_utilization', 0)}%")
        print(f"  最大使用率: {stats.get('max_utilization', 0)}%")
        print(f"  最小使用率: {stats.get('min_utilization', 0)}%")
        print(f"  趋势: {stats.get('trend', 'unknown')}")
        print(f"  数据点数: {stats.get('sample_count', 0)}")
    
    if "chart_data" in cluster_data:
        chart = cluster_data["chart_data"]
        print(f"\n折线图数据:")
        print(f"  标题: {chart.get('title', '')}")
        print(f"  时间点数: {len(chart.get('x_axis', []))}")
        print(f"  数据系列: {len(chart.get('series', []))}")
        for series in chart.get("series", []):
            print(f"    - {series.get('name', '')}: {len(series.get('data', []))} 个数据点")


def print_namespace_analysis(namespace_data: dict):
    """打印namespace分析结果"""
    print("\n" + "="*70)
    print("  【Namespace级别分析】")
    print("="*70)
    
    if "error" in namespace_data:
        print(f"❌ 错误: {namespace_data['error']}")
        return
    
    if "summary" in namespace_data:
        print(f"\n{namespace_data['summary']}")
    
    namespaces = namespace_data.get("namespaces", [])
    if namespaces:
        print(f"\nTop 10 Namespaces (按使用率排序):")
        print(f"{'Namespace':<30} {'平均使用率':<12} {'平均GPU数':<12} {'趋势':<10}")
        print("-" * 70)
        for ns in namespaces[:10]:
            print(f"{ns['namespace']:<30} "
                  f"{ns['avg_utilization']:<11.2f}% "
                  f"{ns['avg_gpu_count']:<12.2f} "
                  f"{ns['trend']:<10}")


def print_low_utilization_annotations(low_util_data: list, summary_data: dict):
    """打印低使用率annotations"""
    print("\n" + "="*70)
    print("  【低使用率Annotations】")
    print("="*70)
    
    if summary_data.get("error"):
        print(f"❌ 错误: {summary_data['error']}")
        return
    
    if not low_util_data:
        print("\n✓ 未发现占用GPU多但使用率低的annotations")
        return
    
    print(f"\n发现 {len(low_util_data)} 个低使用率annotations (使用率<30%, GPU>10):")
    print(f"\n{'Key':<30} {'Value':<30} {'使用率':<10} {'GPU数':<10} {'问题评分':<10}")
    print("-" * 100)
    
    for anno in low_util_data[:20]:
        print(f"{anno['annotation_key']:<30} "
              f"{anno['annotation_value']:<30} "
              f"{anno['avg_utilization']:<9.2f}% "
              f"{anno['avg_gpu_count']:<10.2f} "
              f"{anno['issue_score']:<10.2f}")
    
    if "all_annotations_summary" in summary_data:
        total = summary_data["all_annotations_summary"].get("total_count", 0)
        print(f"\n总共分析了 {total} 个annotations")


def print_workload_table(workload_data: dict):
    """打印workload表格"""
    print("\n" + "="*70)
    print("  【相关Workloads】")
    print("="*70)
    
    if "error" in workload_data:
        print(f"❌ 错误: {workload_data['error']}")
        return
    
    if "summary" in workload_data:
        print(f"\n{workload_data['summary']}")
    
    table_data = workload_data.get("table_data", [])
    if not table_data:
        print("\n未找到相关workloads")
        return
    
    print(f"\n相关Workloads列表:")
    print(f"{'Annotation':<40} {'Workload名称':<30} {'Namespace':<20} {'Kind':<15} {'GPU':<5}")
    print("-" * 120)
    
    for row in table_data[:30]:
        anno_str = f"{row['annotation_key']}:{row['annotation_value']}"
        print(f"{anno_str:<40} "
              f"{row['workload_name']:<30} "
              f"{row['workload_namespace']:<20} "
              f"{row['workload_kind']:<15} "
              f"{row['workload_gpu_allocated']:<5}")


def main():
    """主函数"""
    print("\n" + "="*70)
    print("  GPU Usage Agent - 增强版本示例")
    print("="*70)
    
    # 初始化LLM
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
    
    # 执行分析查询
    print("\n" + "-"*70)
    print("执行查询: 分析最近7天的GPU使用情况")
    print("-"*70)
    
    try:
        result = agent.chat("分析最近7天的GPU使用情况")
        
        print(f"\n【总体摘要】")
        print(result['answer'])
        
        # 打印各部分分析结果
        data = result.get('data', {})
        
        if 'cluster_analysis' in data:
            print_cluster_analysis(data['cluster_analysis'])
        
        if 'namespace_analysis' in data:
            print_namespace_analysis(data['namespace_analysis'])
        
        if 'low_utilization_annotations' in data:
            print_low_utilization_annotations(
                data['low_utilization_annotations'],
                data
            )
        
        if 'workload_table' in data:
            print_workload_table(data['workload_table'])
        
        # 保存完整结果到文件（可选）
        print("\n" + "-"*70)
        print("保存完整结果到 analysis_result.json")
        with open("analysis_result.json", "w", encoding="utf-8") as f:
            json.dump(result, f, ensure_ascii=False, indent=2)
        print("✓ 已保存")
        
    except Exception as e:
        print(f"\n❌ 查询失败: {e}")
        logger.exception("查询异常")
    
    print("\n" + "="*70)
    print("  示例完成")
    print("="*70 + "\n")


if __name__ == "__main__":
    main()

