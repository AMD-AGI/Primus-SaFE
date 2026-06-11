/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package view

import (
	"sort"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func makeUser(name string, ts time.Time) v1.User {
	return v1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.NewTime(ts),
		},
	}
}

// TestUserSliceSort verifies Len, Swap and Less via sort.Sort.
func TestUserSliceSort(t *testing.T) {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	users := UserSlice{
		makeUser("b", base.Add(time.Hour)),
		makeUser("c", base), // same time as "a", sorted by name
		makeUser("a", base),
	}
	if users.Len() != 3 {
		t.Fatalf("Len = %d, want 3", users.Len())
	}

	sort.Sort(users)

	if users[0].Name != "a" || users[1].Name != "c" || users[2].Name != "b" {
		t.Errorf("unexpected order: %s, %s, %s", users[0].Name, users[1].Name, users[2].Name)
	}

	users.Swap(0, 2)
	if users[0].Name != "b" || users[2].Name != "a" {
		t.Errorf("Swap failed: %s, %s", users[0].Name, users[2].Name)
	}
}
