/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

const TableNameEmailOutbox = "email_outbox"

const (
	EmailOutboxStatusPending    = "pending"
	EmailOutboxStatusDispatched = "dispatched"
	EmailOutboxStatusSent       = "sent"
	EmailOutboxStatusFailed     = "failed"

	EmailOutboxSourceSafe = "safe"
	EmailOutboxSourceLens = "lens"
)

type EmailOutbox struct {
	ID           int32          `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	Source       string         `gorm:"column:source;default:safe" json:"source"`
	Recipients   StringArray    `gorm:"column:recipients;type:text[]" json:"recipients"`
	Subject      string         `gorm:"column:subject" json:"subject"`
	HTMLContent  string         `gorm:"column:html_content" json:"html_content"`
	Status       string         `gorm:"column:status;default:pending" json:"status"`
	CreatedAt    time.Time      `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	SentAt       *time.Time     `gorm:"column:sent_at" json:"sent_at,omitempty"`
	ErrorMessage *string        `gorm:"column:error_message" json:"error_message,omitempty"`
}

func (*EmailOutbox) TableName() string {
	return TableNameEmailOutbox
}

func (e *EmailOutbox) BeforeCreate(tx *gorm.DB) error {
	if e.Status == "" {
		e.Status = EmailOutboxStatusPending
	}
	if e.Source == "" {
		e.Source = EmailOutboxSourceSafe
	}
	return nil
}

// StringArray implements driver.Valuer and sql.Scanner for PostgreSQL text[].
type StringArray []string

func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	// PostgreSQL text[] literal format: {item1,item2}
	escaped := make([]string, len(a))
	for i, s := range a {
		s = strings.ReplaceAll(s, `\`, `\\`)
		s = strings.ReplaceAll(s, `"`, `\"`)
		escaped[i] = `"` + s + `"`
	}
	return "{" + strings.Join(escaped, ",") + "}", nil
}

func (a *StringArray) Scan(value interface{}) error {
	if value == nil {
		*a = nil
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return a.parsePostgresArray(string(v))
	case string:
		return a.parsePostgresArray(v)
	default:
		return errors.New("unsupported type for StringArray")
	}
}

func (a *StringArray) parsePostgresArray(s string) error {
	if len(s) >= 2 && s[0] == '[' {
		return json.Unmarshal([]byte(s), a)
	}
	if len(s) < 2 || s[0] != '{' || s[len(s)-1] != '}' {
		*a = nil
		return nil
	}
	inner := s[1 : len(s)-1]
	if inner == "" {
		*a = StringArray{}
		return nil
	}
	var result []string
	var current []byte
	inQuote := false
	escaped := false
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		if escaped {
			current = append(current, c)
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '"' {
			inQuote = !inQuote
			continue
		}
		if c == ',' && !inQuote {
			result = append(result, string(current))
			current = current[:0]
			continue
		}
		current = append(current, c)
	}
	result = append(result, string(current))
	*a = result
	return nil
}
