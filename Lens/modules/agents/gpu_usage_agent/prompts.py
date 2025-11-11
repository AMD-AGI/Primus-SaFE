"""Prompt Templates for GPU Usage Analysis Agent - Enhanced."""

# ============================================================================
# Understanding Phase - Intent Recognition and Entity Extraction
# ============================================================================

UNDERSTAND_PROMPT = """You are a GPU utilization analysis expert. Please analyze the user's question and identify the dimensions and parameters that need to be queried.

User Question: {user_query}

Agent's Supported Analysis Features:
1. **Cluster-level Trend Analysis**: Analyze GPU utilization and allocation rate trends for the entire cluster, provide line chart data
2. **Namespace-level Analysis**: Analyze GPU usage and trends for each namespace
3. **User Occupancy Analysis**: Analyze GPU occupancy and utilization for each user (based on annotation key "primus-safe.user.name"), identify users with high occupancy but low utilization
4. **Low Utilization Resource Identification**: Find other annotations with high GPU occupancy but low utilization
5. **Comparative Analysis**: Compare data across different time periods

Please analyze the user's question and extract the following information:

1. **Time Range** (time_range):
   - Format: {{"type": "relative", "value": "7d"}} or {{"type": "absolute", "start": "2025-01-01", "end": "2025-01-07"}}
   - Relative time: 1d=1 day, 7d=7 days, 30d=30 days
   - If user doesn't specify, default to 7 days

2. **Analysis Type** (analysis_type):
   - "cluster_trend": Cluster-level trend analysis (utilization and allocation rate line charts)
   - "namespace_analysis": Namespace-level analysis
   - "user_analysis": User occupancy analysis (analyze primus-safe.user.name)
   - "low_utilization": Find low utilization resources
   - "full": Complete analysis (includes all features)
   - If user's question is unclear, default to "full"

3. **Specific Dimension** (specific_dimension):
   - If user specifies a specific namespace or username, extract it
   - Format: {{"type": "namespace", "value": "ml-team"}} or {{"type": "user", "value": "zhangsan"}}

4. **Output Format Requirements** (output_format):
   - "chart": Need line chart
   - "table": Need table
   - "both": Need both
   - Default to "both"

5. **Need Clarification**:
   - Only set to true when query intent is completely unclear or critical information is completely missing
   - For general queries (like "analyze GPU usage"), should be set to false and use default parameters

Please return results in strict JSON format (do not add any other text):

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

Example 1 - Cluster Trend Analysis:
User: "What's the trend of cluster GPU utilization and allocation rate in the last 7 days? Give me a line chart"
Return:
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

Example 2 - User Occupancy Analysis:
User: "Analyze which users occupy many GPUs but have low utilization"
Return:
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

Example 3 - Namespace Analysis:
User: "How's the ml-team namespace usage in the last 30 days"
Return:
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

Example 4 - Specific User Analysis:
User: "How's zhangsan's GPU occupancy?"
Return:
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

Example 5 - Complete Analysis:
User: "Analyze recent GPU usage"
Return:
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

Example 6 - Need Clarification:
User: "GPU"
Return:
{{
  "entities": {{}},
  "needs_clarification": true,
  "clarification_question": "What information about GPU would you like to know?\\n1. View cluster overall utilization trends (line chart)\\n2. Analyze usage for each namespace\\n3. View GPU occupancy and utilization for each user (table)\\n4. Find resources with high occupancy but low utilization\\nPlease tell me which type of analysis you need?"
}}

Example 7 - Time Unclear but Inferable:
User: "Check cluster utilization"
Return:
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
# Response Generation Phase - Generate User-Friendly Responses
# ============================================================================

# ============================================================================
# Cluster Trend Analysis Phase - In-depth Cluster Trend Analysis
# ============================================================================

CLUSTER_TREND_ANALYSIS_PROMPT = """You are a GPU cluster utilization analysis expert. Please deeply analyze trends based on the provided cluster statistics and evaluate whether the cluster has utilization issues.

## Cluster Statistics

### Utilization Statistics
- Average Utilization: {avg_utilization}%
- Maximum Utilization: {max_utilization}%
- Minimum Utilization: {min_utilization}%
- Trend: {trend}
- Time Range: {time_range_days} days
- Sample Count: {sample_count}

### Allocation Rate Statistics
- Average Allocation: {avg_allocation}%
- Maximum Allocation: {max_allocation}%
- Minimum Allocation: {min_allocation}%

### Data Point Details
{data_points_summary}

## Analysis Requirements

Please conduct in-depth analysis from the following aspects:

1. **Trend Assessment**
   - Analyze the change trend of utilization and allocation rate (rising, falling, stable, fluctuating)
   - Identify if there are obvious periodic patterns (e.g., weekdays vs weekends)
   - Evaluate trend stability and predictability

2. **Utilization Issue Diagnosis**
   - Evaluate if current average utilization level is reasonable (reference: >70% healthy, 50-70% acceptable, <50% low, <30% severe waste)
   - Analyze the gap between utilization and allocation rate (if allocation is high but utilization is low, indicates serious resource idling)
   - Identify if there are abnormal peaks or valleys
   - Determine if fluctuation range is too large (gap between max and min values)

3. **Resource Utilization Efficiency Evaluation**
   - Comprehensively evaluate overall resource utilization efficiency based on utilization and allocation
   - Calculate resource waste degree (allocated but not fully used portion)
   - Evaluate if there are issues with over-allocation or under-allocation

4. **Problem Severity Rating**
   - Provide an overall score (0-100, 100 means perfect, 0 means severe waste)
   - Clearly indicate if there are issues that need immediate resolution
   - List discovered issues by priority

5. **Optimization Recommendations**
   - Provide specific, actionable optimization suggestions for discovered issues
   - Recommend resource allocation strategy or usage adjustments
   - Provide improvement targets (expected utilization levels)

## Output Format

Please return analysis results in strict JSON format (do not add any other text):

{{
  "trend_analysis": {{
    "trend_description": "Trend description (2-3 sentences)",
    "trend_type": "rising/falling/stable/fluctuating",
    "has_periodicity": true/false,
    "periodicity_description": "Periodicity description (if any)"
  }},
  "utilization_issues": {{
    "has_issues": true/false,
    "issue_level": "severe/moderate/mild/none",
    "utilization_assessment": "Utilization level assessment",
    "allocation_gap": "Gap analysis between allocation and utilization",
    "volatility_assessment": "Volatility assessment"
  }},
  "efficiency_evaluation": {{
    "overall_score": 85,
    "efficiency_level": "excellent/good/fair/poor/very poor",
    "waste_percentage": 15.5,
    "resource_allocation_status": "reasonable/over-allocated/under-allocated"
  }},
  "problem_severity": {{
    "overall_score": 85,
    "needs_immediate_action": false,
    "critical_issues": ["Issue 1", "Issue 2"],
    "warnings": ["Warning 1", "Warning 2"]
  }},
  "recommendations": [
    {{
      "priority": "high/medium/low",
      "issue": "Issue description",
      "suggestion": "Specific suggestion",
      "expected_improvement": "Expected improvement effect"
    }}
  ],
  "summary": "Summarize overall evaluation and most important recommendations in 2-3 sentences"
}}

Example Output:
{{
  "trend_analysis": {{
    "trend_description": "Over the past 7 days, the cluster GPU utilization has remained stable around 45% on average, but allocation rate is as high as 80%, indicating many GPUs are allocated but not fully used.",
    "trend_type": "stable",
    "has_periodicity": false,
    "periodicity_description": null
  }},
  "utilization_issues": {{
    "has_issues": true,
    "issue_level": "moderate",
    "utilization_assessment": "Average utilization of 45% is at a low level, with obvious resource waste issues.",
    "allocation_gap": "The gap between 80% allocation and 45% utilization reaches 35 percentage points, indicating about 44% of allocated GPU resources are idle.",
    "volatility_assessment": "Utilization fluctuates from 20% to 75%, with large volatility and unstable resource usage."
  }},
  "efficiency_evaluation": {{
    "overall_score": 56,
    "efficiency_level": "fair",
    "waste_percentage": 43.75,
    "resource_allocation_status": "over-allocated"
  }},
  "problem_severity": {{
    "overall_score": 56,
    "needs_immediate_action": true,
    "critical_issues": ["Severe resource waste: 80% allocation but only 45% utilization", "Excessive utilization fluctuation, lack of stability"],
    "warnings": ["Recommend optimizing resource allocation strategy", "Need to identify low-utilization users and tasks"]
  }},
  "recommendations": [
    {{
      "priority": "high",
      "issue": "Gap between allocation and utilization too large (35 percentage points)",
      "suggestion": "Identify and optimize low-utilization GPU allocations, consider introducing GPU sharing or dynamic reclaim mechanisms",
      "expected_improvement": "Expected to increase average utilization to 60-70%, reducing resource waste by about 20-30%"
    }},
    {{
      "priority": "medium",
      "issue": "Excessive utilization fluctuation",
      "suggestion": "Analyze low-utilization periods, adjust task scheduling strategy, smooth resource usage curve",
      "expected_improvement": "Improve resource usage stability, reduce peak-valley difference"
    }}
  ],
  "summary": "Overall cluster resource utilization efficiency is fair (56 points). Main issue is 80% allocation but only 45% utilization, with about 44% resource waste. Priority recommendation is to identify low-utilization tasks and optimize resource allocation strategy, expected to increase utilization to 60-70%."
}}
"""


RESPONSE_GENERATION_PROMPT = """You are a GPU utilization analysis expert. Please generate a clear, professional report based on the analysis results.

User Question: {user_query}

Analysis Type: {analysis_type}

Analysis Results:
{analysis_results}

Please generate a report including the following:
1. **Summary**: Summarize key findings in 1-2 sentences
2. **Detailed Analysis**: Provide specific analysis based on data
3. **Recommendations**: If issues are found (e.g., low utilization), provide optimization suggestions

Notes:
- Use clear, professional language
- Highlight key data and findings
- For percentage data, keep 2 decimal places
- If there's chart data, state "Line chart has been generated, please check visualization results"
- If there's table data, state "Detailed data shown in the table below"

Please return the report in markdown format:
"""
