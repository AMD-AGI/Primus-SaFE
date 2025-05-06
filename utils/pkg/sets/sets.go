/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package sets

type Set map[string]struct{}

func NewSet() Set {
	return make(Set)
}

func NewSetByKeys(keys ...string) Set {
	set := NewSet()
	set.Insert(keys...)
	return set
}

func (s Set) Insert(keys ...string) Set {
	for _, key := range keys {
		s[key] = struct{}{}
	}
	return s
}

func (s Set) Delete(keys ...string) Set {
	for _, key := range keys {
		delete(s, key)
	}
	return s
}

func (s Set) Has(key string) bool {
	if s == nil {
		return false
	}
	_, ok := s[key]
	return ok
}

func (s Set) Len() int {
	return len(s)
}

func (s Set) Clear() Set {
	keysToDelete := s.UnsortedList()
	return s.Delete(keysToDelete...)
}

func (s Set) Clone() Set {
	result := make(Set, len(s))
	for key := range s {
		result.Insert(key)
	}
	return result
}

// Difference returns a set of objects that are not in s2.
// For example:
// s1 = {a1, a2, a3}
// s2 = {a1, a2, a4, a5}
// s1.Difference(s2) = {a3}
// s2.Difference(s1) = {a4, a5}
func (s Set) Difference(s2 Set) Set {
	result := NewSet()
	for key := range s {
		if !s2.Has(key) {
			result.Insert(key)
		}
	}
	return result
}

// Union returns a new set which includes items in either s1 or s2.
// For example:
// s1 = {a1, a2}
// s2 = {a3, a4}
// s1.Union(s2) = {a1, a2, a3, a4}
// s2.Union(s1) = {a1, a2, a3, a4}
func (s Set) Union(s2 Set) Set {
	result := s.Clone()
	for key := range s2 {
		result.Insert(key)
	}
	return result
}

// Intersection returns a new set which includes the item in BOTH s1 and s2
// For example:
// s1 = {a1, a2}
// s2 = {a2, a3}
// s1.Intersection(s2) = {a2}
func (s Set) Intersection(s2 Set) Set {
	var walk, other Set
	result := NewSet()
	if s.Len() < s2.Len() {
		walk = s
		other = s2
	} else {
		walk = s2
		other = s
	}
	for key := range walk {
		if other.Has(key) {
			result.Insert(key)
		}
	}
	return result
}

func (s Set) Equal(s2 Set) bool {
	if len(s) != len(s2) {
		return false
	}
	for key := range s2 {
		if !s.Has(key) {
			return false
		}
	}
	return true
}

func (s Set) UnsortedList() []string {
	results := make([]string, 0, s.Len())
	for k := range s {
		results = append(results, k)
	}
	return results
}
