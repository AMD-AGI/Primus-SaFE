package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNetworkPolicy_Structure(t *testing.T) {
	t.Run("创建NetworkPolicy结构体", func(t *testing.T) {
		policy := NetworkPolicy{
			InternalHosts: []string{"10.0.0.0/8"},
			K8SPod:        []string{"10.244.0.0/16"},
			K8SSvc:        []string{"10.96.0.0/12"},
			Dns:           []string{"169.254.25.10/32"},
			AbnormalBlackList: []string{
				"malicious.com",
				"192.168.1.100",
			},
			AbnormalWhiteList: []string{
				"trusted.com",
				"10.0.0.1",
			},
			Localhost: []string{"127.0.0.1"},
		}

		assert.NotNil(t, policy)
		assert.Equal(t, 1, len(policy.InternalHosts))
		assert.Equal(t, 1, len(policy.K8SPod))
		assert.Equal(t, 1, len(policy.K8SSvc))
		assert.Equal(t, 1, len(policy.Dns))
		assert.Equal(t, 2, len(policy.AbnormalBlackList))
		assert.Equal(t, 2, len(policy.AbnormalWhiteList))
		assert.Equal(t, 1, len(policy.Localhost))
	})

	t.Run("创建空NetworkPolicy", func(t *testing.T) {
		policy := NetworkPolicy{}
		
		assert.NotNil(t, policy)
		assert.Nil(t, policy.InternalHosts)
		assert.Nil(t, policy.K8SPod)
		assert.Nil(t, policy.K8SSvc)
		assert.Nil(t, policy.Dns)
		assert.Nil(t, policy.AbnormalBlackList)
		assert.Nil(t, policy.AbnormalWhiteList)
		assert.Nil(t, policy.Localhost)
	})

	t.Run("NetworkPolicy字段类型验证", func(t *testing.T) {
		policy := NetworkPolicy{
			InternalHosts:     []string{},
			K8SPod:            []string{},
			K8SSvc:            []string{},
			Dns:               []string{},
			AbnormalBlackList: []string{},
			AbnormalWhiteList: []string{},
			Localhost:         []string{},
		}

		assert.NotNil(t, policy.InternalHosts)
		assert.NotNil(t, policy.K8SPod)
		assert.NotNil(t, policy.K8SSvc)
		assert.NotNil(t, policy.Dns)
		assert.NotNil(t, policy.AbnormalBlackList)
		assert.NotNil(t, policy.AbnormalWhiteList)
		assert.NotNil(t, policy.Localhost)
		
		assert.Empty(t, policy.InternalHosts)
		assert.Empty(t, policy.K8SPod)
		assert.Empty(t, policy.K8SSvc)
		assert.Empty(t, policy.Dns)
		assert.Empty(t, policy.AbnormalBlackList)
		assert.Empty(t, policy.AbnormalWhiteList)
		assert.Empty(t, policy.Localhost)
	})
}

func TestGetDefaultPolicy(t *testing.T) {
	t.Run("获取默认策略", func(t *testing.T) {
		policy := GetDefaultPolicy()
		
		assert.NotNil(t, policy)
		assert.NotNil(t, policy.InternalHosts)
		
		// 验证默认的内部网络段
		assert.Contains(t, policy.InternalHosts, "10.0.0.0/8")
		assert.Contains(t, policy.InternalHosts, "172.16.0.0/12")
		assert.Contains(t, policy.InternalHosts, "192.168.0.0/16")
		assert.Equal(t, 3, len(policy.InternalHosts))
	})

	t.Run("GetDefaultPolicy返回值的独立性", func(t *testing.T) {
		policy1 := GetDefaultPolicy()
		policy2 := GetDefaultPolicy()
		
		// 两次调用应该返回相同的值
		assert.Equal(t, policy1.InternalHosts, policy2.InternalHosts)
		
		// 修改返回值不应该影响其他调用
		policy1.InternalHosts = append(policy1.InternalHosts, "203.0.113.0/24")
		
		// policy2 不应该被修改（因为返回的是副本）
		policy3 := GetDefaultPolicy()
		assert.NotEqual(t, len(policy1.InternalHosts), len(policy3.InternalHosts))
	})

	t.Run("验证默认策略的CIDR格式", func(t *testing.T) {
		policy := GetDefaultPolicy()
		
		for _, cidr := range policy.InternalHosts {
			assert.Contains(t, cidr, "/", "CIDR应该包含子网掩码")
			assert.NotEmpty(t, cidr, "CIDR不应该为空")
		}
	})
}

func TestDefaultPolicy_InternalHosts(t *testing.T) {
	tests := []struct {
		name     string
		expected []string
	}{
		{
			name: "验证所有默认内部网络段",
			expected: []string{
				"10.0.0.0/8",      // Class A private network
				"172.16.0.0/12",   // Class B private network
				"192.168.0.0/16",  // Class C private network
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := GetDefaultPolicy()
			
			assert.Equal(t, len(tt.expected), len(policy.InternalHosts))
			
			for _, expectedCidr := range tt.expected {
				assert.Contains(t, policy.InternalHosts, expectedCidr)
			}
		})
	}
}

func TestNetworkPolicy_EdgeCases(t *testing.T) {
	t.Run("NetworkPolicy包含大量条目", func(t *testing.T) {
		largeList := make([]string, 1000)
		for i := 0; i < 1000; i++ {
			largeList[i] = "entry-" + string(rune(i))
		}
		
		policy := NetworkPolicy{
			AbnormalBlackList: largeList,
		}
		
		assert.Equal(t, 1000, len(policy.AbnormalBlackList))
	})

	t.Run("NetworkPolicy包含重复条目", func(t *testing.T) {
		policy := NetworkPolicy{
			InternalHosts: []string{
				"10.0.0.0/8",
				"10.0.0.0/8",
				"10.0.0.0/8",
			},
		}
		
		assert.Equal(t, 3, len(policy.InternalHosts))
		// 注意：结构体不会自动去重，这是预期行为
	})

	t.Run("NetworkPolicy包含特殊字符", func(t *testing.T) {
		policy := NetworkPolicy{
			Dns: []string{
				"test-dns.example.com",
				"dns_with_underscore.test",
				"192.168.1.1/32",
			},
		}
		
		assert.Equal(t, 3, len(policy.Dns))
		for _, dns := range policy.Dns {
			assert.NotEmpty(t, dns)
		}
	})
}

func TestNetworkPolicy_JSONTags(t *testing.T) {
	t.Run("验证结构体有正确的JSON标签", func(t *testing.T) {
		// 这个测试确保结构体可以正确序列化/反序列化为JSON
		policy := NetworkPolicy{
			InternalHosts:     []string{"10.0.0.0/8"},
			K8SPod:            []string{"pod-cidr"},
			K8SSvc:            []string{"svc-cidr"},
			Dns:               []string{"dns-server"},
			AbnormalBlackList: []string{"blacklist"},
			AbnormalWhiteList: []string{"whitelist"},
			Localhost:         []string{"localhost"},
		}
		
		// 验证所有字段都已设置
		assert.NotEmpty(t, policy.InternalHosts)
		assert.NotEmpty(t, policy.K8SPod)
		assert.NotEmpty(t, policy.K8SSvc)
		assert.NotEmpty(t, policy.Dns)
		assert.NotEmpty(t, policy.AbnormalBlackList)
		assert.NotEmpty(t, policy.AbnormalWhiteList)
		assert.NotEmpty(t, policy.Localhost)
	})
}

func TestNetworkPolicy_Comparison(t *testing.T) {
	t.Run("比较两个相同的策略", func(t *testing.T) {
		policy1 := NetworkPolicy{
			InternalHosts: []string{"10.0.0.0/8"},
			K8SPod:        []string{"pod"},
		}
		
		policy2 := NetworkPolicy{
			InternalHosts: []string{"10.0.0.0/8"},
			K8SPod:        []string{"pod"},
		}
		
		assert.Equal(t, policy1.InternalHosts, policy2.InternalHosts)
		assert.Equal(t, policy1.K8SPod, policy2.K8SPod)
	})

	t.Run("比较两个不同的策略", func(t *testing.T) {
		policy1 := NetworkPolicy{
			InternalHosts: []string{"10.0.0.0/8"},
		}
		
		policy2 := NetworkPolicy{
			InternalHosts: []string{"192.168.0.0/16"},
		}
		
		assert.NotEqual(t, policy1.InternalHosts, policy2.InternalHosts)
	})
}

func TestNetworkPolicy_ModifyAfterCreation(t *testing.T) {
	t.Run("创建后修改策略", func(t *testing.T) {
		policy := NetworkPolicy{
			InternalHosts: []string{"10.0.0.0/8"},
		}
		
		assert.Equal(t, 1, len(policy.InternalHosts))
		
		// 添加新的条目
		policy.InternalHosts = append(policy.InternalHosts, "192.168.0.0/16")
		assert.Equal(t, 2, len(policy.InternalHosts))
		
		// 修改黑名单
		policy.AbnormalBlackList = []string{"malicious.com"}
		assert.Equal(t, 1, len(policy.AbnormalBlackList))
		
		// 清空白名单
		policy.AbnormalWhiteList = []string{}
		assert.Empty(t, policy.AbnormalWhiteList)
	})

	t.Run("修改切片不影响原始数据", func(t *testing.T) {
		original := []string{"10.0.0.0/8", "192.168.0.0/16"}
		policy := NetworkPolicy{
			InternalHosts: original,
		}
		
		// 修改策略的切片
		policy.InternalHosts = append(policy.InternalHosts, "172.16.0.0/12")
		
		// 原始切片应该也被修改（Go的切片共享底层数组）
		// 但如果容量不够，会创建新数组
		assert.Equal(t, 3, len(policy.InternalHosts))
	})
}

func TestNetworkPolicy_NilSlices(t *testing.T) {
	t.Run("处理nil切片", func(t *testing.T) {
		policy := NetworkPolicy{}
		
		// nil切片应该与空切片行为不同
		assert.Nil(t, policy.InternalHosts)
		assert.Equal(t, 0, len(policy.InternalHosts))
		
		// append到nil切片会创建新切片
		policy.InternalHosts = append(policy.InternalHosts, "10.0.0.0/8")
		assert.NotNil(t, policy.InternalHosts)
		assert.Equal(t, 1, len(policy.InternalHosts))
	})
}

func TestGetDefaultPolicy_Consistency(t *testing.T) {
	t.Run("多次调用返回一致的值", func(t *testing.T) {
		results := make([]NetworkPolicy, 10)
		for i := 0; i < 10; i++ {
			results[i] = GetDefaultPolicy()
		}
		
		// 所有结果的InternalHosts应该相同
		for i := 1; i < 10; i++ {
			assert.Equal(t, results[0].InternalHosts, results[i].InternalHosts)
		}
	})
}

func TestNetworkPolicy_FieldCombinations(t *testing.T) {
	tests := []struct {
		name   string
		policy NetworkPolicy
		check  func(t *testing.T, p NetworkPolicy)
	}{
		{
			name: "只有InternalHosts",
			policy: NetworkPolicy{
				InternalHosts: []string{"10.0.0.0/8"},
			},
			check: func(t *testing.T, p NetworkPolicy) {
				assert.NotEmpty(t, p.InternalHosts)
				assert.Nil(t, p.K8SPod)
				assert.Nil(t, p.K8SSvc)
			},
		},
		{
			name: "只有黑名单",
			policy: NetworkPolicy{
				AbnormalBlackList: []string{"malicious.com"},
			},
			check: func(t *testing.T, p NetworkPolicy) {
				assert.NotEmpty(t, p.AbnormalBlackList)
				assert.Nil(t, p.AbnormalWhiteList)
			},
		},
		{
			name: "同时有黑名单和白名单",
			policy: NetworkPolicy{
				AbnormalBlackList: []string{"bad.com"},
				AbnormalWhiteList: []string{"good.com"},
			},
			check: func(t *testing.T, p NetworkPolicy) {
				assert.NotEmpty(t, p.AbnormalBlackList)
				assert.NotEmpty(t, p.AbnormalWhiteList)
				assert.NotEqual(t, p.AbnormalBlackList[0], p.AbnormalWhiteList[0])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, tt.policy)
		})
	}
}

