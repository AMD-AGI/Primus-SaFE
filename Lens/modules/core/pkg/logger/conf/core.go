package conf

type Core string

const (
	ZapCore    Core = "zap"
	LogrusCore      = "logrus"
)

func isValidCore(c Core) bool {
	return (c == ZapCore) ||
		(c == LogrusCore)
}
