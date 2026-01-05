package airegistry

import (
	"context"
	"sync"
)

// Router routes topics to agents
type Router struct {
	registry Registry
	cache    *routeCache
}

// routeCache caches topic to agent mappings
type routeCache struct {
	mu      sync.RWMutex
	routes  map[string]*AgentRegistration
	enabled bool
}

// NewRouter creates a new topic router
func NewRouter(registry Registry) *Router {
	return &Router{
		registry: registry,
		cache: &routeCache{
			routes:  make(map[string]*AgentRegistration),
			enabled: true,
		},
	}
}

// Route finds the best agent for a topic
func (r *Router) Route(ctx context.Context, topic string) (*AgentRegistration, error) {
	// Try cache first
	if agent := r.cache.get(topic); agent != nil {
		// Verify agent is still healthy
		if agent.Status == AgentStatusHealthy || agent.Status == AgentStatusUnknown {
			return agent, nil
		}
		// Cache entry is stale, remove it
		r.cache.remove(topic)
	}

	// Query registry
	agent, err := r.registry.GetHealthyAgentForTopic(ctx, topic)
	if err != nil {
		return nil, err
	}

	// Update cache
	r.cache.set(topic, agent)

	return agent, nil
}

// RouteAll returns all agents that can handle a topic
func (r *Router) RouteAll(ctx context.Context, topic string) ([]*AgentRegistration, error) {
	return r.registry.ListByTopic(ctx, topic)
}

// InvalidateCache clears the routing cache
func (r *Router) InvalidateCache() {
	r.cache.clear()
}

// InvalidateCacheForTopic clears cache for a specific topic
func (r *Router) InvalidateCacheForTopic(topic string) {
	r.cache.remove(topic)
}

// InvalidateCacheForAgent clears cache entries for an agent
func (r *Router) InvalidateCacheForAgent(agentName string) {
	r.cache.removeByAgent(agentName)
}

// DisableCache disables the routing cache
func (r *Router) DisableCache() {
	r.cache.mu.Lock()
	defer r.cache.mu.Unlock()
	r.cache.enabled = false
	r.cache.routes = make(map[string]*AgentRegistration)
}

// EnableCache enables the routing cache
func (r *Router) EnableCache() {
	r.cache.mu.Lock()
	defer r.cache.mu.Unlock()
	r.cache.enabled = true
}

// get retrieves a cached route
func (c *routeCache) get(topic string) *AgentRegistration {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.enabled {
		return nil
	}

	return c.routes[topic]
}

// set caches a route
func (c *routeCache) set(topic string, agent *AgentRegistration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.enabled {
		return
	}

	// Make a copy
	copy := *agent
	c.routes[topic] = &copy
}

// remove removes a cached route
func (c *routeCache) remove(topic string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.routes, topic)
}

// removeByAgent removes all cached routes for an agent
func (c *routeCache) removeByAgent(agentName string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for topic, agent := range c.routes {
		if agent.Name == agentName {
			delete(c.routes, topic)
		}
	}
}

// clear clears all cached routes
func (c *routeCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.routes = make(map[string]*AgentRegistration)
}

// TopicMatcher provides utilities for topic pattern matching
type TopicMatcher struct{}

// Match checks if a topic matches a pattern
func (m *TopicMatcher) Match(pattern, topic string) bool {
	return matchTopicPattern(pattern, topic)
}

// MatchAny checks if a topic matches any of the patterns
func (m *TopicMatcher) MatchAny(patterns []string, topic string) bool {
	for _, pattern := range patterns {
		if matchTopicPattern(pattern, topic) {
			return true
		}
	}
	return false
}

// ExtractDomain extracts the domain from a topic (first segment)
// e.g., "alert.advisor.aggregate" -> "alert"
func ExtractDomain(topic string) string {
	for i, c := range topic {
		if c == '.' {
			return topic[:i]
		}
	}
	return topic
}

// ExtractAgent extracts the agent from a topic (second segment)
// e.g., "alert.advisor.aggregate" -> "advisor"
func ExtractAgent(topic string) string {
	firstDot := -1
	for i, c := range topic {
		if c == '.' {
			if firstDot == -1 {
				firstDot = i
			} else {
				return topic[firstDot+1 : i]
			}
		}
	}
	if firstDot != -1 {
		return topic[firstDot+1:]
	}
	return ""
}

// ExtractAction extracts the action from a topic (third segment)
// e.g., "alert.advisor.aggregate-workloads" -> "aggregate-workloads"
func ExtractAction(topic string) string {
	dotCount := 0
	lastDot := -1
	for i, c := range topic {
		if c == '.' {
			dotCount++
			lastDot = i
		}
	}
	if dotCount >= 2 && lastDot != -1 {
		return topic[lastDot+1:]
	}
	return ""
}
