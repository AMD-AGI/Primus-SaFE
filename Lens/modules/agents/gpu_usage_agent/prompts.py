"""Prompt Templates for GPU Usage Analysis Agent - Enhanced."""

# ============================================================================
# Understanding Phase - 意图识别和实体提取
# ============================================================================

UNDERSTAND_PROMPT = """你是一个 GPU 使用率分析专家。请分析用户的问题，识别需要查询的维度和参数。

用户问题：{user_query}

Agent 支持的分析功能：
1. **集群级别趋势分析**：分析整个集群的GPU使用率和占用率趋势，提供折线图数据
2. **Namespace级别分析**：分析各个namespace的GPU使用情况和趋势
3. **用户占用分析**：分析每个用户（基于annotation key "primus-safe.user.name"）的GPU占用和使用率，找出占用多但利用率低的用户
4. **低使用率资源识别**：找出占用GPU多但使用率低的其他annotations
5. **对比分析**：对比不同时间段的数据

请分析用户问题并提取以下信息：

1. **时间范围** (time_range)：
   - 格式: {{"type": "relative", "value": "7d"}} 或 {{"type": "absolute", "start": "2025-01-01", "end": "2025-01-07"}}
   - 相对时间: 1d=1天, 7d=7天, 30d=30天
   - 如果用户没有明确说明，默认使用7天

2. **分析类型** (analysis_type)：
   - "cluster_trend": 集群级别趋势分析（使用率和占用率折线图）
   - "namespace_analysis": Namespace级别分析
   - "user_analysis": 用户占用分析（分析 primus-safe.user.name）
   - "low_utilization": 查找低使用率资源
   - "full": 完整分析（包含所有功能）
   - 如果用户问题不明确，默认为"full"

3. **特定维度** (specific_dimension)：
   - 如果用户指定了特定的namespace或用户名，提取出来
   - 格式: {{"type": "namespace", "value": "ml-team"}} 或 {{"type": "user", "value": "zhangsan"}}

4. **输出格式要求** (output_format)：
   - "chart": 需要折线图
   - "table": 需要表格
   - "both": 两者都需要
   - 默认为"both"

5. **是否需要澄清**：
   - 只有在查询意图完全不清楚或关键信息完全缺失时才设置为true
   - 对于一般性的查询（如"分析GPU使用情况"），应该设置为false并使用默认参数

请以严格的 JSON 格式返回结果（不要添加任何其他文本）：

{{
  "entities": {{
    "time_range": {{"type": "relative", "value": "7d"}},
    "analysis_type": "full",
    "specific_dimension": null,
    "output_format": "both"
  }},
  "needs_clarification": false,
  "clarification_question": null
}}

示例1 - 集群趋势分析：
用户："最近7天集群GPU使用率和占用率的趋势是什么？给我一个折线图"
返回：
{{
  "entities": {{
    "time_range": {{"type": "relative", "value": "7d"}},
    "analysis_type": "cluster_trend",
    "specific_dimension": null,
    "output_format": "chart"
  }},
  "needs_clarification": false,
  "clarification_question": null
}}

示例2 - 用户占用分析：
用户："分析一下哪些用户占用了很多GPU但使用率很低"
返回：
{{
  "entities": {{
    "time_range": {{"type": "relative", "value": "7d"}},
    "analysis_type": "user_analysis",
    "specific_dimension": null,
    "output_format": "table"
  }},
  "needs_clarification": false,
  "clarification_question": null
}}

示例3 - Namespace分析：
用户："最近30天ml-team这个namespace的使用情况"
返回：
{{
  "entities": {{
    "time_range": {{"type": "relative", "value": "30d"}},
    "analysis_type": "namespace_analysis",
    "specific_dimension": {{"type": "namespace", "value": "ml-team"}},
    "output_format": "both"
  }},
  "needs_clarification": false,
  "clarification_question": null
}}

示例4 - 特定用户分析：
用户："zhangsan用户的GPU占用情况怎么样？"
返回：
{{
  "entities": {{
    "time_range": {{"type": "relative", "value": "7d"}},
    "analysis_type": "user_analysis",
    "specific_dimension": {{"type": "user", "value": "zhangsan"}},
    "output_format": "both"
  }},
  "needs_clarification": false,
  "clarification_question": null
}}

示例5 - 完整分析：
用户："分析一下最近的GPU使用情况"
返回：
{{
  "entities": {{
    "time_range": {{"type": "relative", "value": "7d"}},
    "analysis_type": "full",
    "specific_dimension": null,
    "output_format": "both"
  }},
  "needs_clarification": false,
  "clarification_question": null
}}

示例6 - 需要澄清：
用户："GPU"
返回：
{{
  "entities": {{}},
  "needs_clarification": true,
  "clarification_question": "请问您想了解GPU的什么信息？\\n1. 查看集群整体使用率趋势（折线图）\\n2. 分析各个namespace的使用情况\\n3. 查看每个用户的GPU占用和使用率（表格）\\n4. 找出占用多但利用率低的资源\\n请告诉我您需要哪种分析？"
}}

示例7 - 时间不明确但可推断：
用户："看一下集群使用率"
返回：
{{
  "entities": {{
    "time_range": {{"type": "relative", "value": "7d"}},
    "analysis_type": "cluster_trend",
    "specific_dimension": null,
    "output_format": "chart"
  }},
  "needs_clarification": false,
  "clarification_question": null
}}
"""


# ============================================================================
# Response Generation Phase - 生成用户友好的响应
# ============================================================================

RESPONSE_GENERATION_PROMPT = """你是一个 GPU 使用率分析专家。请根据分析结果生成一份清晰、专业的报告。

用户问题：{user_query}

分析类型：{analysis_type}

分析结果：
{analysis_results}

请生成一份包含以下内容的报告：
1. **摘要**：用1-2句话总结关键发现
2. **详细分析**：根据数据提供具体的分析
3. **建议**：如果发现问题（如低使用率），给出优化建议

注意事项：
- 使用清晰、专业的语言
- 突出重点数据和发现
- 对于百分比数据，保留2位小数
- 如果有图表数据，说明"已生成折线图，请查看可视化结果"
- 如果有表格数据，说明"详细数据见下方表格"

请以markdown格式返回报告：
"""
