#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
JSON解析兼容性测试脚本

用于验证 safe_json_parse 函数对各种格式JSON的处理能力
"""

import sys
import io

# 确保在Windows上也能正常输出中文
if sys.platform == 'win32':
    sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8')

from utils import safe_json_parse


def test_safe_json_parse():
    """测试 safe_json_parse 函数的各种情况"""
    
    print("=" * 80)
    print("JSON 解析兼容性测试")
    print("=" * 80)
    
    # 测试用例
    test_cases = [
        {
            "name": "标准JSON",
            "input": '{"key": "value", "number": 123}',
            "expected_valid": True
        },
        {
            "name": "前后带空白字符的JSON",
            "input": '\n\n\n{"key": "value"}\n\n',
            "expected_valid": True
        },
        {
            "name": "前后带多个换行和空格的JSON（模拟LLM返回）",
            "input": '''


{

  "intent": ["trend"],

  "entities": {

    "time_range": {"type": "relative", "value": "1d"},

    "dimension": "cluster",

    "dimension_value": "x-flannel",

    "metric": "utilization"

  }

}

''',
            "expected_valid": True
        },
        {
            "name": "包含额外文本的JSON",
            "input": 'Here is the result: {"key": "value"} End of result',
            "expected_valid": True
        },
        {
            "name": "无效的JSON",
            "input": 'This is not JSON at all',
            "expected_valid": False
        },
        {
            "name": "空字符串",
            "input": '',
            "expected_valid": False
        },
        {
            "name": "只有空白字符",
            "input": '\n\n   \t\t  \n',
            "expected_valid": False
        },
        {
            "name": "不完整的JSON",
            "input": '{"key": "value"',
            "expected_valid": False
        }
    ]
    
    # 运行测试
    passed = 0
    failed = 0
    
    for i, test_case in enumerate(test_cases, 1):
        print(f"\n测试 {i}: {test_case['name']}")
        print("-" * 80)
        print(f"输入: {repr(test_case['input'][:100])}{'...' if len(test_case['input']) > 100 else ''}")
        
        result = safe_json_parse(test_case['input'])
        is_valid = result is not None
        
        if is_valid == test_case['expected_valid']:
            print(f"✅ 通过 - 结果: {result}")
            passed += 1
        else:
            print(f"❌ 失败 - 期望: {'有效' if test_case['expected_valid'] else '无效'}, 实际: {'有效' if is_valid else '无效'}")
            if result:
                print(f"   实际结果: {result}")
            failed += 1
    
    # 汇总
    print("\n" + "=" * 80)
    print(f"测试汇总: 总计 {len(test_cases)} 个测试, 通过 {passed} 个, 失败 {failed} 个")
    print("=" * 80)
    
    return failed == 0


def test_real_world_example():
    """测试真实世界的示例（用户报告的问题）"""
    
    print("\n" + "=" * 80)
    print("真实案例测试 - 用户报告的问题")
    print("=" * 80)
    
    # 用户报告的真实案例
    problematic_json = '''```json



{

  "intent": ["trend"],

  "entities": {

    "time_range": {"type": "relative", "value": "1d"},

    "dimension": "cluster",

    "dimension_value": "x-flannel",

    "metric": "utilization",

    "granularity": "day",

    "analysis_depth": "shallow"

  },

  "needs_clarification": false,

  "missing_info": [],

  "clarification_question": null,

  "should_fetch_metadata": false,

  "metadata_to_fetch": [],

  "understanding": "用户想查看x-flannel集群最近1天的使用率趋势报告"

}

```'''
    
    print(f"原始输入（前200字符）: {repr(problematic_json[:200])}...")
    
    result = safe_json_parse(problematic_json)
    
    if result:
        print(f"\n✅ 解析成功!")
        print(f"\n解析结果:")
        print(f"  - intent: {result.get('intent')}")
        print(f"  - dimension: {result.get('entities', {}).get('dimension')}")
        print(f"  - dimension_value: {result.get('entities', {}).get('dimension_value')}")
        print(f"  - metric: {result.get('entities', {}).get('metric')}")
        print(f"  - understanding: {result.get('understanding')}")
        return True
    else:
        print("\n❌ 解析失败")
        return False


if __name__ == "__main__":
    print("\n")
    
    # 运行基础测试
    basic_tests_passed = test_safe_json_parse()
    
    # 运行真实案例测试
    real_world_test_passed = test_real_world_example()
    
    # 最终结果
    print("\n" + "=" * 80)
    if basic_tests_passed and real_world_test_passed:
        print("🎉 所有测试通过！JSON解析兼容性增强功能正常工作。")
    else:
        print("⚠️ 部分测试失败，请检查实现。")
    print("=" * 80)
    print()

