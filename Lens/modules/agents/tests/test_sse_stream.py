"""测试 SSE 流式接口"""

import requests
import json
import time


def test_sse_stream():
    """测试流式聊天接口"""
    
    url = "http://localhost:8001/api/gpu-analysis/chat/stream"
    
    payload = {
        "query": "最近7天的GPU使用率如何？",
        "save_history": True
    }
    
    print("=" * 60)
    print("开始测试 SSE 流式接口")
    print("=" * 60)
    print(f"URL: {url}")
    print(f"Query: {payload['query']}")
    print("=" * 60)
    
    try:
        response = requests.post(url, json=payload, stream=True, timeout=300)
        
        if response.status_code != 200:
            print(f"错误: HTTP {response.status_code}")
            print(response.text)
            return
        
        print("\n开始接收流式数据...\n")
        
        event_count = 0
        start_time = time.time()
        
        for line in response.iter_lines():
            if line:
                line_str = line.decode('utf-8')
                
                # 解析 SSE 格式
                if line_str.startswith('data: '):
                    json_str = line_str[6:]
                    try:
                        data = json.loads(json_str)
                        event_count += 1
                        
                        event_type = data.get('type')
                        timestamp = time.time() - start_time
                        
                        print(f"[{timestamp:.2f}s] 事件 #{event_count} - 类型: {event_type}")
                        
                        if event_type == 'session':
                            print(f"  Session ID: {data.get('session_id')}")
                        
                        elif event_type == 'status':
                            print(f"  阶段: {data.get('stage')}")
                            print(f"  消息: {data.get('message')}")
                        
                        elif event_type == 'data':
                            print(f"  阶段: {data.get('stage')}")
                            print(f"  消息: {data.get('message')}")
                            # 显示数据键
                            if 'data' in data:
                                data_keys = list(data['data'].keys())
                                print(f"  数据键: {data_keys}")
                        
                        elif event_type == 'final':
                            print(f"  答案: {data.get('answer')}")
                            if 'data' in data:
                                data_keys = list(data['data'].keys())
                                print(f"  最终数据键: {data_keys}")
                            print("\n✅ 流式响应完成!")
                        
                        elif event_type == 'error':
                            print(f"  ❌ 错误类型: {data.get('error_type')}")
                            print(f"  错误消息: {data.get('error_message')}")
                        
                        elif event_type == 'done':
                            print(f"\n✅ 流结束")
                            break
                        
                        print()
                        
                    except json.JSONDecodeError as e:
                        print(f"  ⚠️ JSON 解析错误: {e}")
                        print(f"  原始数据: {json_str[:200]}")
        
        end_time = time.time()
        duration = end_time - start_time
        
        print("=" * 60)
        print(f"测试完成!")
        print(f"总事件数: {event_count}")
        print(f"总耗时: {duration:.2f} 秒")
        print(f"平均每个事件: {duration/event_count:.2f} 秒")
        print("=" * 60)
        
    except requests.exceptions.RequestException as e:
        print(f"\n❌ 请求错误: {e}")
    except KeyboardInterrupt:
        print("\n\n⚠️ 用户中断")


def test_non_stream():
    """测试非流式接口（对比）"""
    
    url = "http://localhost:8001/api/gpu-analysis/chat"
    
    payload = {
        "query": "最近7天的GPU使用率如何？",
        "save_history": True
    }
    
    print("\n" + "=" * 60)
    print("开始测试非流式接口（对比）")
    print("=" * 60)
    print(f"URL: {url}")
    print(f"Query: {payload['query']}")
    print("=" * 60)
    
    try:
        start_time = time.time()
        print("\n发送请求...")
        
        response = requests.post(url, json=payload, timeout=300)
        
        end_time = time.time()
        duration = end_time - start_time
        
        if response.status_code == 200:
            result = response.json()
            print(f"\n✅ 请求成功!")
            print(f"答案: {result.get('answer', '')[:100]}...")
            print(f"总耗时: {duration:.2f} 秒")
        else:
            print(f"\n❌ 请求失败: HTTP {response.status_code}")
            print(response.text)
        
        print("=" * 60)
        
    except requests.exceptions.RequestException as e:
        print(f"\n❌ 请求错误: {e}")


if __name__ == "__main__":
    import sys
    
    if len(sys.argv) > 1 and sys.argv[1] == "--non-stream":
        test_non_stream()
    else:
        test_sse_stream()
        
        # 可选：也测试非流式接口进行对比
        if len(sys.argv) > 1 and sys.argv[1] == "--both":
            print("\n\n")
            test_non_stream()

