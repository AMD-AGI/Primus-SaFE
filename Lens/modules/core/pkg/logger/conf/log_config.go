package conf

type LogConfig struct {
	Core            Core      `json:"core" yaml:"core"`
	Level           Level     `json:"level" yaml:"level"`
	Formatter       Formatter `json:"formatter" yaml:"formatter"`
	Outputs         []Output  `json:"outputs" yaml:"outputs"`
	SetReportCaller bool      `json:"set_report_caller" yaml:"set_report_caller"`
}

func (l *LogConfig) SetLevel(level Level) *LogConfig {
	l.Level = level
	return l
}

func DefaultConfig() *LogConfig {
	return &LogConfig{
		Core:      LogrusCore,
		Level:     DebugLevel,
		Formatter: ConsoleFormater,
		Outputs: []Output{
			{
				Type: OutputTypeStdout,
			},
		},
		SetReportCaller: false,
	}
}
