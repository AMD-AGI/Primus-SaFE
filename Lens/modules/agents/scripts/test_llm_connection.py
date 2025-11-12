"""测试LLM连接和诊断问题

这个脚本用于测试LLM API连接，输出详细的错误信息
"""

import os
import sys
import logging
import traceback

# 设置详细日志
logging.basicConfig(
    level=logging.DEBUG,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

# 为OpenAI设置DEBUG级别
openai_logger = logging.getLogger("openai")
openai_logger.setLevel(logging.DEBUG)

httpx_logger = logging.getLogger("httpx")
httpx_logger.setLevel(logging.DEBUG)

def test_openai_connection():
    """测试OpenAI连接"""
    print("\n" + "="*80)
    print("测试 OpenAI API 连接")
    print("="*80 + "\n")
    
    try:
        from langchain_openai import ChatOpenAI
        from langchain_core.messages import SystemMessage
        
        # 从环境变量读取配置
        api_key = os.getenv("OPENAI_API_KEY", "")
        base_url = os.getenv("OPENAI_BASE_URL", None)
        model = os.getenv("OPENAI_MODEL", "gpt-4")
        
        print(f"配置信息:")
        print(f"  API Key: {api_key[:10]}... (隐藏)" if api_key else "  API Key: 未设置")
        print(f"  Base URL: {base_url if base_url else '使用默认URL'}")
        print(f"  Model: {model}")
        print()
        
        if not api_key:
            print("❌ 错误: OPENAI_API_KEY 环境变量未设置")
            print("请设置环境变量:")
            print("  export OPENAI_API_KEY='your-api-key'")
            return False
        
        print("正在初始化 ChatOpenAI 客户端...")
        llm = ChatOpenAI(
            model=model,
            api_key=api_key,
            base_url=base_url,
            temperature=0,
            max_tokens=100,
            timeout=30
        )
        
        print("✓ 客户端初始化成功")
        print()
        
        print("正在发送测试请求...")
        messages = [SystemMessage(content="请用一句话介绍你自己。")]
        
        response = llm.invoke(messages)
        
        print("✓ 请求成功!")
        print(f"\n响应内容: {response.content[:200]}...\n")
        
        return True
        
    except Exception as e:
        error_type = type(e).__name__
        error_msg = str(e)
        error_traceback = traceback.format_exc()
        
        print("\n" + "="*80)
        print("❌ 连接失败")
        print("="*80)
        print(f"\n错误类型: {error_type}")
        print(f"错误消息: {error_msg}")
        print(f"\n完整堆栈跟踪:\n{error_traceback}")
        print("="*80 + "\n")
        
        # 常见问题诊断
        print("可能的原因:")
        if "APIConnectionError" in error_type or "Connection" in str(e):
            print("  1. 网络连接问题")
            print("  2. Base URL配置错误")
            print("  3. 防火墙或代理设置")
        elif "AuthenticationError" in error_type or "401" in str(e):
            print("  1. API Key 错误或过期")
            print("  2. API Key 格式不正确")
        elif "RateLimitError" in error_type or "429" in str(e):
            print("  1. API调用频率超限")
            print("  2. 账户额度不足")
        elif "APIError" in error_type or "500" in str(e):
            print("  1. OpenAI服务器错误")
            print("  2. 请稍后重试")
        
        print("\n建议:")
        print("  1. 检查环境变量设置")
        print("  2. 确认API Key有效性")
        print("  3. 检查网络连接")
        print("  4. 查看OpenAI状态页面: https://status.openai.com/")
        print()
        
        return False


def test_config_loading():
    """测试配置加载"""
    print("\n" + "="*80)
    print("测试配置加载")
    print("="*80 + "\n")
    
    try:
        # 添加父目录到路径
        sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..'))
        
        from config import load_config, get_config
        
        print("正在加载配置文件...")
        config = load_config()
        
        print("✓ 配置加载成功")
        print()
        
        # 显示关键配置
        print("关键配置项:")
        print(f"  LLM Provider: {get_config('llm.provider', 'N/A')}")
        print(f"  LLM Model: {get_config('llm.model', 'N/A')}")
        print(f"  LLM Base URL: {get_config('llm.base_url', '默认')}")
        print(f"  Lens API URL: {get_config('lens.api_url', 'N/A')}")
        print(f"  Cache Enabled: {get_config('cache.enabled', False)}")
        print()
        
        return True
        
    except Exception as e:
        print(f"❌ 配置加载失败: {str(e)}")
        print(f"\n完整堆栈跟踪:\n{traceback.format_exc()}")
        return False


def test_lens_api_connection():
    """测试Lens API连接"""
    print("\n" + "="*80)
    print("测试 Lens API 连接")
    print("="*80 + "\n")
    
    try:
        import requests
        
        # 从环境变量或配置文件读取
        api_url = os.getenv("LENS_API_URL", "http://localhost:30182")
        
        print(f"Lens API URL: {api_url}")
        print()
        
        print("正在测试连接...")
        
        # 测试clusters接口
        test_url = f"{api_url}/v1/gpu-aggregation/clusters"
        response = requests.get(test_url, timeout=5)
        
        print(f"✓ 连接成功 (状态码: {response.status_code})")
        
        if response.status_code == 200:
            data = response.json()
            print(f"响应数据: {data}")
        
        print()
        return True
        
    except requests.exceptions.ConnectionError as e:
        print(f"❌ 连接失败: 无法连接到 {api_url}")
        print(f"错误: {str(e)}")
        print("\n建议:")
        print("  1. 检查Lens API服务是否运行")
        print("  2. 确认URL配置正确")
        print("  3. 检查防火墙设置")
        print()
        return False
        
    except Exception as e:
        print(f"❌ 测试失败: {str(e)}")
        print(f"\n完整堆栈跟踪:\n{traceback.format_exc()}")
        return False


def main():
    """主函数"""
    print("\n" + "="*80)
    print("GPU Usage Analysis Agent - 连接诊断工具")
    print("="*80)
    
    results = []
    
    # 1. 测试配置加载
    results.append(("配置加载", test_config_loading()))
    
    # 2. 测试LLM连接
    results.append(("LLM连接", test_openai_connection()))
    
    # 3. 测试Lens API连接
    results.append(("Lens API连接", test_lens_api_connection()))
    
    # 显示总结
    print("\n" + "="*80)
    print("诊断总结")
    print("="*80 + "\n")
    
    for name, success in results:
        status = "✓ 通过" if success else "❌ 失败"
        print(f"  {name:20} {status}")
    
    all_passed = all(success for _, success in results)
    
    if all_passed:
        print("\n🎉 所有测试通过！Agent应该可以正常工作。")
    else:
        print("\n⚠️  部分测试失败，请根据上述错误信息进行排查。")
    
    print()


if __name__ == "__main__":
    main()

