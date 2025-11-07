#!/usr/bin/env python3
"""
GPU 使用率下降根因分析示例

本示例展示如何使用新增的 get_available_dimension_values 方法
来分析集群 GPU 使用率下降的根本原因。
"""

import json
import sys
from typing import Dict, List, Tuple
from datetime import datetime, timedelta

# 假设已经安装了必要的依赖
sys.path.insert(0, '..')
from tools import GPUAnalysisTools
from utils import safe_json_parse


class UsageRootCauseAnalyzer:
    """GPU 使用率下降根因分析器"""
    
    def __init__(self, api_base_url: str, cluster_name: str = None):
        """
        初始化分析器
        
        Args:
            api_base_url: API 基础 URL
            cluster_name: 集群名称（可选）
        """
        self.tools = GPUAnalysisTools(api_base_url, cluster_name)
        self.cluster_name = cluster_name
    
    def analyze_cluster_trend(self, time_range_days: int = 7) -> Dict:
        """
        分析集群级别的使用率趋势
        
        Returns:
            包含趋势信息的字典
        """
        print(f"📊 步骤 1: 分析集群整体使用率趋势（最近 {time_range_days} 天）...")
        
        result = self.tools.query_gpu_usage_trend(
            dimension="cluster",
            granularity="day",
            time_range_days=time_range_days,
            metric_type="utilization"
        )
        
        data = safe_json_parse(result)
        if not data:
            print("   ⚠️ 无法解析数据")
            return {}
        stats = data.get("statistics", {})
        
        print(f"   平均使用率: {stats.get('average', 0):.2f}%")
        print(f"   最高使用率: {stats.get('max', 0):.2f}%")
        print(f"   最低使用率: {stats.get('min', 0):.2f}%")
        print(f"   趋势: {stats.get('trend', 'unknown')}")
        
        return data
    
    def analyze_by_namespace(self, time_range_days: int = 7) -> List[Tuple[str, float]]:
        """
        按 namespace 分析使用率
        
        Returns:
            (namespace, 平均使用率) 的列表，按使用率升序排列
        """
        print(f"\n📦 步骤 2: 按 Namespace 分析...")
        
        # 获取所有 namespaces
        namespaces_result = self.tools.get_available_namespaces(time_range_days)
        namespaces_data = safe_json_parse(namespaces_result)
        if not namespaces_data:
            print("   ⚠️ 无法解析 namespaces 数据")
            return []
        namespaces = namespaces_data.get('namespaces', [])
        
        print(f"   发现 {len(namespaces)} 个 namespaces")
        
        # 查询每个 namespace 的使用率
        namespace_stats = []
        for ns in namespaces:
            result = self.tools.query_gpu_usage_trend(
                dimension="namespace",
                dimension_value=ns,
                granularity="day",
                time_range_days=time_range_days,
                metric_type="utilization"
            )
            
            data = safe_json_parse(result)
            if not data:
                continue
            avg_util = data.get("statistics", {}).get("average", 0)
            namespace_stats.append((ns, avg_util))
            print(f"     - {ns}: {avg_util:.2f}%")
        
        # 按使用率排序（从低到高）
        namespace_stats.sort(key=lambda x: x[1])
        
        return namespace_stats
    
    def analyze_by_dimension(
        self, 
        dimension_type: str, 
        time_range_days: int = 7,
        top_n: int = 5
    ) -> List[Tuple[str, str, float]]:
        """
        按 label 或 annotation 分析使用率
        
        Args:
            dimension_type: "label" 或 "annotation"
            time_range_days: 时间范围
            top_n: 返回使用率最低的前 N 个
        
        Returns:
            (key, value, 平均使用率) 的列表，按使用率升序排列
        """
        print(f"\n🏷️  步骤 3: 按 {dimension_type.upper()} 分析...")
        
        # 获取所有 dimension keys
        keys_result = self.tools.get_available_dimension_keys(
            dimension_type=dimension_type,
            time_range_days=time_range_days
        )
        keys_data = safe_json_parse(keys_result)
        if not keys_data:
            print(f"   ⚠️ 无法解析 {dimension_type} keys 数据")
            return []
        dimension_keys = keys_data.get('dimension_keys', [])
        
        print(f"   发现 {len(dimension_keys)} 个 {dimension_type} keys")
        
        # 对每个 key，获取所有 values 并查询使用率
        dimension_stats = []
        for key in dimension_keys:
            print(f"\n   分析 {dimension_type} key: {key}")
            
            # 【新功能】获取该 key 的所有 values
            values_result = self.tools.get_available_dimension_values(
                dimension_type=dimension_type,
                dimension_key=key,
                time_range_days=time_range_days
            )
            values_data = safe_json_parse(values_result)
            if not values_data:
                print(f"     ⚠️ 无法解析 {key} 的 values 数据")
                continue
            values = values_data.get('dimension_values', [])
            
            print(f"     发现 {len(values)} 个不同的 values")
            
            # 查询每个 value 的使用率
            for value in values[:10]:  # 限制每个 key 最多查 10 个 values
                result = self.tools.query_gpu_usage_trend(
                    dimension="label" if dimension_type == "label" else "annotation",
                    dimension_value=f"{key}:{value}",
                    granularity="day",
                    time_range_days=time_range_days,
                    metric_type="utilization"
                )
                
                data = safe_json_parse(result)
                if data and 'error' not in data:
                    avg_util = data.get("statistics", {}).get("average", 0)
                    dimension_stats.append((key, value, avg_util))
                    print(f"       - {value}: {avg_util:.2f}%")
        
        # 按使用率排序（从低到高）
        dimension_stats.sort(key=lambda x: x[2])
        
        # 返回使用率最低的前 N 个
        return dimension_stats[:top_n]
    
    def generate_report(
        self,
        cluster_data: Dict,
        namespace_stats: List[Tuple[str, float]],
        label_stats: List[Tuple[str, str, float]],
        annotation_stats: List[Tuple[str, str, float]]
    ):
        """
        生成分析报告
        """
        print("\n" + "="*80)
        print("📈 GPU 使用率下降根因分析报告")
        print("="*80)
        
        # 集群整体情况
        stats = cluster_data.get("statistics", {})
        print(f"\n【集群整体情况】")
        print(f"  平均使用率: {stats.get('average', 0):.2f}%")
        print(f"  趋势: {stats.get('trend', 'unknown')}")
        
        if stats.get('trend') == 'decreasing':
            print(f"  ⚠️  使用率呈下降趋势！")
        
        # Namespace 分析
        print(f"\n【Namespace 使用率最低的前 3 名】")
        for i, (ns, util) in enumerate(namespace_stats[:3], 1):
            print(f"  {i}. {ns}: {util:.2f}%")
        
        # Label 分析
        print(f"\n【Label 使用率最低的前 3 名】")
        if label_stats:
            for i, (key, value, util) in enumerate(label_stats[:3], 1):
                print(f"  {i}. {key}={value}: {util:.2f}%")
        else:
            print("  无数据")
        
        # Annotation 分析
        print(f"\n【Annotation 使用率最低的前 3 名】")
        if annotation_stats:
            for i, (key, value, util) in enumerate(annotation_stats[:3], 1):
                print(f"  {i}. {key}={value}: {util:.2f}%")
        else:
            print("  无数据")
        
        # 根因推断
        print(f"\n【可能的根因】")
        all_low_util = []
        all_low_util.extend([(f"namespace:{ns}", util) for ns, util in namespace_stats[:3]])
        all_low_util.extend([(f"label:{k}={v}", util) for k, v, util in label_stats[:3]])
        all_low_util.extend([(f"annotation:{k}={v}", util) for k, v, util in annotation_stats[:3]])
        
        # 按使用率排序
        all_low_util.sort(key=lambda x: x[1])
        
        for i, (dimension, util) in enumerate(all_low_util[:5], 1):
            print(f"  {i}. {dimension} 的平均使用率仅为 {util:.2f}%")
            print(f"     建议检查该维度下的任务是否存在资源浪费或配置问题")
        
        print("\n" + "="*80)


def main():
    """主函数"""
    # 配置
    API_BASE_URL = "http://localhost:8080"
    CLUSTER_NAME = "default"  # 可选
    TIME_RANGE_DAYS = 7
    
    print("🚀 开始 GPU 使用率下降根因分析...")
    print(f"   API: {API_BASE_URL}")
    print(f"   集群: {CLUSTER_NAME or '(默认)'}")
    print(f"   时间范围: 最近 {TIME_RANGE_DAYS} 天")
    
    # 创建分析器
    analyzer = UsageRootCauseAnalyzer(API_BASE_URL, CLUSTER_NAME)
    
    try:
        # 步骤 1: 分析集群趋势
        cluster_data = analyzer.analyze_cluster_trend(TIME_RANGE_DAYS)
        
        # 步骤 2: 按 namespace 分析
        namespace_stats = analyzer.analyze_by_namespace(TIME_RANGE_DAYS)
        
        # 步骤 3: 按 label 分析
        label_stats = analyzer.analyze_by_dimension("label", TIME_RANGE_DAYS, top_n=5)
        
        # 步骤 4: 按 annotation 分析
        annotation_stats = analyzer.analyze_by_dimension("annotation", TIME_RANGE_DAYS, top_n=5)
        
        # 生成报告
        analyzer.generate_report(
            cluster_data,
            namespace_stats,
            label_stats,
            annotation_stats
        )
        
    except Exception as e:
        print(f"\n❌ 分析过程中出现错误: {e}")
        import traceback
        traceback.print_exc()
        return 1
    
    print("\n✅ 分析完成！")
    return 0


if __name__ == "__main__":
    sys.exit(main())

