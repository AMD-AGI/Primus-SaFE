/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

const (
	// teamsCacheTTL is how long a team answer of the identity service is reused.
	teamsCacheTTL = 30 * time.Second
	// teamsFailureTTL is how long a failed call is remembered, so an unreachable service is not
	// asked again by every request.
	teamsFailureTTL = 10 * time.Second
	// teamsCallTimeout bounds one display call.
	teamsCallTimeout = 3 * time.Second
	maxTeamsBody     = 4 << 20

	teamsKey       = "teams"
	teamMembersKey = "members:"
)

// TeamMember is a member of a team as the identity service describes it.
type TeamMember struct {
	UserId        string `json:"userId"`
	Email         string `json:"email,omitempty"`
	Name          string `json:"name,omitempty"`
	PreferredName string `json:"preferredName,omitempty"`
	Role          string `json:"role,omitempty"`
}

// TeamInfo is the display data of a team: its names, member count and managers.
type TeamInfo struct {
	Id          string       `json:"id"`
	DisplayName string       `json:"displayName,omitempty"`
	Description string       `json:"description,omitempty"`
	MemberCount int          `json:"memberCount"`
	Managers    []TeamMember `json:"managers,omitempty"`
}

// teamsEntry is one cached answer or failure.
type teamsEntry struct {
	teams   map[string]TeamInfo
	members []TeamMember
	err     error
	fetched time.Time
}

var (
	teamsMu    sync.Mutex
	teamsCache = map[string]*teamsEntry{}
)

// ResetTeamsCache drops every cached team answer and failure.
func ResetTeamsCache() {
	teamsMu.Lock()
	teamsCache = map[string]*teamsEntry{}
	teamsMu.Unlock()
}

// ListTeams returns the teams of the identity service by team id. The answer is cached briefly.
// It is display data, never a reason to grant or refuse access; an error is an API status
// (see getTeamsBody) that the caller may answer with.
func ListTeams(ctx context.Context) (map[string]TeamInfo, error) {
	e, err := cachedTeamsCall(ctx, teamsKey, "teams", func(body []byte, e *teamsEntry) error {
		teams, perr := parseTeams(body)
		e.teams = teams
		return perr
	})
	if err != nil {
		return nil, err
	}
	return e.teams, nil
}

// ListTeamMembers returns the members of a team of the identity service. The answer is cached
// briefly. It is display data with the errors of ListTeams.
func ListTeamMembers(ctx context.Context, team string) ([]TeamMember, error) {
	if team == "" {
		return nil, fmt.Errorf("the team id is empty")
	}
	e, err := cachedTeamsCall(ctx, teamMembersKey+team, "teams/"+url.PathEscape(team)+"/members",
		func(body []byte, e *teamsEntry) error {
			members, perr := parseTeamMembers(body)
			e.members = members
			return perr
		})
	if err != nil {
		return nil, err
	}
	return e.members, nil
}

// cachedTeamsCall returns the cached entry of key, or calls the identity service at path and
// stores what parse makes of the answer; an answer parse cannot read is a 502. A failure is cached
// for a shorter time than an answer.
func cachedTeamsCall(ctx context.Context, key, path string,
	parse func(body []byte, e *teamsEntry) error) (*teamsEntry, error) {
	teamsMu.Lock()
	cached := teamsCache[key]
	teamsMu.Unlock()
	if cached != nil {
		if cached.err == nil && time.Since(cached.fetched) < teamsCacheTTL {
			return cached, nil
		}
		if cached.err != nil && time.Since(cached.fetched) < teamsFailureTTL {
			return nil, cached.err
		}
	}
	entry := &teamsEntry{fetched: time.Now()}
	body, err := getTeamsBody(ctx, path)
	if err == nil {
		if perr := parse(body, entry); perr != nil {
			err = badGateway(fmt.Sprintf("identity service answered GET %s with an unexpected body: %v", path, perr))
		}
	}
	if err != nil {
		entry.err = err
		klog.ErrorS(err, "failed to read teams from the identity service", "path", path)
	}
	teamsMu.Lock()
	if len(teamsCache) > 1024 {
		teamsCache = map[string]*teamsEntry{}
	}
	teamsCache[key] = entry
	teamsMu.Unlock()
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// getTeamsBody sends GET /api/v1/<path> to the identity service as the introspection client,
// which holds the identity.teams.read scope, and returns the body of a 200 answer. A failure is
// the error mapCall makes of it (503 when the service cannot be asked or fails, 502 when it
// refuses), and 404 when the service does not know the path.
func getTeamsBody(ctx context.Context, path string) ([]byte, error) {
	body, status, err := callTeams(ctx, path)
	switch {
	case err != nil:
		return nil, mapCall("GET "+path, 0, err)
	case status == http.StatusNotFound:
		return nil, apierrors.NewNotFound(schema.GroupResource{Resource: "teams"}, path)
	case status != http.StatusOK:
		return nil, mapCall("GET "+path, status, nil)
	}
	return body, nil
}

// callTeams sends GET /api/v1/<path> to the identity service as the introspection client and
// returns the body and status of the answer.
func callTeams(ctx context.Context, path string) ([]byte, int, error) {
	base, err := ResolveBase()
	if err != nil {
		return nil, 0, err
	}
	clientID, secret, err := introspectionSecret()
	if err != nil {
		return nil, 0, err
	}
	endpoint, err := url.JoinPath(base, "api/v1", path)
	if err != nil {
		return nil, 0, &ConfigError{Msg: "cannot build the teams URL"}
	}
	cli, err := httpClient()
	if err != nil {
		return nil, 0, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, teamsCallTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(clientID, secret)
	resp, err := cli.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTeamsBody))
	if err != nil {
		return nil, 0, err
	}
	return body, resp.StatusCode, nil
}

// teamList and teamMemberList are the answers of GET /api/v1/teams and GET /api/v1/teams/{teamId}/members.
type teamList struct {
	Items *[]teamItem `json:"items"`
}

type teamMemberList struct {
	Items *[]TeamMember `json:"items"`
}

// teamItem is a team of GET /api/v1/teams; the managers are user ids.
type teamItem struct {
	TeamId      string   `json:"teamId"`
	DisplayName string   `json:"displayName"`
	Description string   `json:"description"`
	MemberCount int      `json:"memberCount"`
	Managers    []string `json:"managers"`
}

// parseTeams reads the team list, an object holding the teams as "items". Teams without an id
// are left out.
func parseTeams(body []byte) (map[string]TeamInfo, error) {
	var list teamList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	if list.Items == nil {
		return nil, fmt.Errorf("the team list has no items")
	}
	result := make(map[string]TeamInfo, len(*list.Items))
	for _, item := range *list.Items {
		if item.TeamId == "" {
			continue
		}
		info := TeamInfo{
			Id:          item.TeamId,
			DisplayName: item.DisplayName,
			Description: item.Description,
			MemberCount: item.MemberCount,
		}
		for _, id := range item.Managers {
			if id != "" {
				info.Managers = append(info.Managers, TeamMember{UserId: id, Role: "manager"})
			}
		}
		result[info.Id] = info
	}
	return result, nil
}

// parseTeamMembers reads the member list, an object holding the members as "items". Members
// without a user id and an e-mail are left out.
func parseTeamMembers(body []byte) ([]TeamMember, error) {
	var list teamMemberList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	if list.Items == nil {
		return nil, fmt.Errorf("the member list has no items")
	}
	var result []TeamMember
	for _, member := range *list.Items {
		if member.UserId == "" && member.Email == "" {
			continue
		}
		result = append(result, member)
	}
	return result, nil
}
