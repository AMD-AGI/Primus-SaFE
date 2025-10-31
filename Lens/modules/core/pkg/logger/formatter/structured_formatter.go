package formatter

import (
	"fmt"
	"github.com/sirupsen/logrus"
)

const (
	StructuredField = "structure_name"
)

type StructuredFormatter struct {
}

func (s *StructuredFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	msg := fmt.Sprintf("[%s][%s][%s] %s\n", entry.Data[StructuredField], entry.Time.Format("2006-01-02 15:04:05"), entry.Level.String(), entry.Message)
	return []byte(msg), nil
}
