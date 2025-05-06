/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package log

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/klog/v2"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/httpclient"
)

const (
	IndexDateFormat = "2006.01.02"
)

var (
	once     sync.Once
	instance *LogClient
)

type LogClient struct {
	username   string
	password   string
	host       string
	port       int
	prefix     string
	httpClient httpclient.Interface
}

func Instance() LogInterface {
	once.Do(func() {
		instance = &LogClient{
			host:       commonconfig.GetLogServiceHost(),
			port:       commonconfig.GetLogServicePort(),
			prefix:     commonconfig.GetLogServicePrefix(),
			username:   commonconfig.GetLogServiceUser(),
			password:   commonconfig.GetLogServicePasswd(),
			httpClient: httpclient.Instance(),
		}
	})
	return instance
}

func (c *LogClient) Request(sinceTime, untilTime time.Time, method, path string, body []byte) ([]byte, error) {
	index, err := c.buildIndex(sinceTime, untilTime)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return c.Process(index+path, method, body)
}

func (c *LogClient) Process(path, method string, body []byte) ([]byte, error) {
	endpoint := c.host + ":" + strconv.Itoa(c.port)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	url := endpoint + path
	klog.Infof("do open search, url: %s, body: %s", url, body)
	req, err := httpclient.BuildRequest(url, method, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("fail to request open search: %s", string(resp.Body))
	}
	return resp.Body, nil
}

func (c *LogClient) buildIndex(sinceTime, untilTime time.Time) (string, error) {
	if sinceTime.Equal(untilTime) {
		return c.prefix + sinceTime.Format(IndexDateFormat), nil
	}

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
