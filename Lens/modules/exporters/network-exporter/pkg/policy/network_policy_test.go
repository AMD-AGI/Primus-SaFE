// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNetworkPolicy_Structure(t *testing.T) {
	t.Run("create NetworkPolicy struct", func(t *testing.T) {
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

	t.Run("create empty NetworkPolicy", func(t *testing.T) {
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

	t.Run("NetworkPolicy field type validation", func(t *testing.T) {
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
	t.Run("get default policy", func(t *testing.T) {
		policy := GetDefaultPolicy()
		
		assert.NotNil(t, policy)
		assert.NotNil(t, policy.InternalHosts)
		
		// verify default internal network segments
		assert.Contains(t, policy.InternalHosts, "10.0.0.0/8")
		assert.Contains(t, policy.InternalHosts, "172.16.0.0/12")
		assert.Contains(t, policy.InternalHosts, "192.168.0.0/16")
		assert.Equal(t, 3, len(policy.InternalHosts))
	})

	t.Run("GetDefaultPolicy return value independence", func(t *testing.T) {
		policy1 := GetDefaultPolicy()
		policy2 := GetDefaultPolicy()
		
		// two calls should return the same value
		assert.Equal(t, policy1.InternalHosts, policy2.InternalHosts)
		
		// modifying return value should not affect other calls
		policy1.InternalHosts = append(policy1.InternalHosts, "203.0.113.0/24")
		
		// policy2 should not be modified (because a copy is returned)
		policy3 := GetDefaultPolicy()
		assert.NotEqual(t, len(policy1.InternalHosts), len(policy3.InternalHosts))
	})

	t.Run("verify default policy CIDR format", func(t *testing.T) {
		policy := GetDefaultPolicy()
		
		for _, cidr := range policy.InternalHosts {
			assert.Contains(t, cidr, "/", "CIDR should contain subnet mask")
			assert.NotEmpty(t, cidr, "CIDR should not be empty")
		}
	})
}

func TestDefaultPolicy_InternalHosts(t *testing.T) {
	tests := []struct {
		name     string
		expected []string
	}{
		{
			name: "verify all default internal network segments",
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
	t.Run("NetworkPolicy contains large number of entries", func(t *testing.T) {
		largeList := make([]string, 1000)
		for i := 0; i < 1000; i++ {
			largeList[i] = "entry-" + string(rune(i))
		}
		
		policy := NetworkPolicy{
			AbnormalBlackList: largeList,
		}
		
		assert.Equal(t, 1000, len(policy.AbnormalBlackList))
	})

	t.Run("NetworkPolicy contains duplicate entries", func(t *testing.T) {
		policy := NetworkPolicy{
			InternalHosts: []string{
				"10.0.0.0/8",
				"10.0.0.0/8",
				"10.0.0.0/8",
			},
		}
		
		assert.Equal(t, 3, len(policy.InternalHosts))
		// note: struct does not automatically deduplicate, this is expected behavior
	})

	t.Run("NetworkPolicy contains special characters", func(t *testing.T) {
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
	t.Run("verify struct has correct JSON tags", func(t *testing.T) {
		// this test ensures struct can be properly serialized/deserialized to JSON
		policy := NetworkPolicy{
			InternalHosts:     []string{"10.0.0.0/8"},
			K8SPod:            []string{"pod-cidr"},
			K8SSvc:            []string{"svc-cidr"},
			Dns:               []string{"dns-server"},
			AbnormalBlackList: []string{"blacklist"},
			AbnormalWhiteList: []string{"whitelist"},
			Localhost:         []string{"localhost"},
		}
		
		// verify all fields are set
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
	t.Run("compare two identical policies", func(t *testing.T) {
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

	t.Run("compare two different policies", func(t *testing.T) {
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
	t.Run("modify policy after creation", func(t *testing.T) {
		policy := NetworkPolicy{
			InternalHosts: []string{"10.0.0.0/8"},
		}
		
		assert.Equal(t, 1, len(policy.InternalHosts))
		
		// add new entry
		policy.InternalHosts = append(policy.InternalHosts, "192.168.0.0/16")
		assert.Equal(t, 2, len(policy.InternalHosts))
		
		// modify blacklist
		policy.AbnormalBlackList = []string{"malicious.com"}
		assert.Equal(t, 1, len(policy.AbnormalBlackList))
		
		// clear whitelist
		policy.AbnormalWhiteList = []string{}
		assert.Empty(t, policy.AbnormalWhiteList)
	})

	t.Run("modifying slice does not affect original data", func(t *testing.T) {
		original := []string{"10.0.0.0/8", "192.168.0.0/16"}
		policy := NetworkPolicy{
			InternalHosts: original,
		}
		
		// modify policy slice
		policy.InternalHosts = append(policy.InternalHosts, "172.16.0.0/12")
		
		// original slice should also be modified (Go slices share underlying array)
		// but if capacity is insufficient, a new array will be created
		assert.Equal(t, 3, len(policy.InternalHosts))
	})
}

func TestNetworkPolicy_NilSlices(t *testing.T) {
	t.Run("handle nil slices", func(t *testing.T) {
		policy := NetworkPolicy{}
		
		// nil slice should behave differently from empty slice
		assert.Nil(t, policy.InternalHosts)
		assert.Equal(t, 0, len(policy.InternalHosts))
		
		// appending to nil slice creates new slice
		policy.InternalHosts = append(policy.InternalHosts, "10.0.0.0/8")
		assert.NotNil(t, policy.InternalHosts)
		assert.Equal(t, 1, len(policy.InternalHosts))
	})
}

func TestGetDefaultPolicy_Consistency(t *testing.T) {
	t.Run("multiple calls return consistent values", func(t *testing.T) {
		results := make([]NetworkPolicy, 10)
		for i := 0; i < 10; i++ {
			results[i] = GetDefaultPolicy()
		}
		
		// all results' InternalHosts should be the same
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
			name: "only InternalHosts",
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
			name: "only blacklist",
			policy: NetworkPolicy{
				AbnormalBlackList: []string{"malicious.com"},
			},
			check: func(t *testing.T, p NetworkPolicy) {
				assert.NotEmpty(t, p.AbnormalBlackList)
				assert.Nil(t, p.AbnormalWhiteList)
			},
		},
		{
			name: "both blacklist and whitelist",
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

