/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package opensearch

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"k8s.io/klog/v2"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/httpclient"
)

const (
	IndexDateFormat = "2006.01.02"
)

var (
	once     sync.Once
	instance *SearchClient
)

type SearchClient struct {
	username   string
	password   string
	endpoint   string
	prefix     string
	httpClient httpclient.Interface
}

func NewClient() *SearchClient {
	once.Do(func() {
		instance = &SearchClient{
			endpoint:   commonconfig.GetLogServiceEndpoint(),
			prefix:     commonconfig.GetLogServicePrefix(),
			username:   commonconfig.GetLogServiceUser(),
			password:   commonconfig.GetLogServicePasswd(),
			httpClient: httpclient.NewHttpClient(),
		}
	})
	return instance
}

func (c *SearchClient) RequestByTimeRange(sinceTime, untilTime time.Time,
	uri, httpMethod string, body []byte) ([]byte, error) {
	index, err := c.getQueryIndex(sinceTime, untilTime)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}
	return c.Request(index+uri, httpMethod, body)
}

func (c *SearchClient) Request(uri, httpMethod string, body []byte) ([]byte, error) {
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}
	url := c.endpoint + uri
	klog.Infof("request to openSearch, url: %s, body: %s", url, body)
	req, err := httpclient.BuildRequest(url, httpMethod, body)
	if err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	req.SetBasicAuth(c.username, c.password)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("failed to request openSearch: %s", string(resp.Body))
	}
	return resp.Body, nil
}

func (c *SearchClient) getQueryIndex(sinceTime, untilTime time.Time) (string, error) {
	if sinceTime.Equal(untilTime) {
		return c.prefix + sinceTime.Format(IndexDateFormat), nil
	}

	// If the time range is too large, use the wildcard * directly
	days := int(untilTime.Sub(sinceTime).Hours() / 24)
	if days >= 30 {
		return c.prefix + "*", nil
	}

	sinceTime = sinceTime.Truncate(time.Hour * 24)
	untilTime = untilTime.Truncate(time.Hour * 24)
	result := ""
	currentDate := sinceTime
	for !currentDate.After(untilTime) {
		if result != "" {
			result += ","
		}
		result += c.prefix + currentDate.Format(IndexDateFormat)
		currentDate = currentDate.AddDate(0, 0, 1)
	}
	return result, nil
}
