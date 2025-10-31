package conf

const (
	OutputTypeStdout     string = "stdout"
	OutputTypeStderr            = "stderr"
	OutputTypeFile              = "file"
	OutputTypeRotateFile        = "rotate_file"
	OutputTypeSyslog            = "syslog"
)

type Output struct {
	Type       string      `json:"type" yaml:"type"` // stdout, stderr, file, rotate_file, syslog
	File       *string     `json:"file" yaml:"file"`
	RotateFile *RotateFile `json:"rotate_file" yaml:"rotate_file"`
}

type RotateFile struct {
	// Following options refer to: https://github.com/natefinch/lumberjack

	// Filename is the file to write logs to.  Backup log files will be retained
	// in the same directory.  It uses <processname>-lumberjack.log in
	// os.TempDir() if empty.
	FileName string `json:"file_name" yaml:"file_name"`

	// MaxSize is the maximum size in megabytes of the log file before it gets rotated. It defaults to 100 megabytes.
	MaxSize int `json:"max_size" yaml:"max_size"`

	// MaxAge is the maximum number of days to retain old log files based on the
	// timestamp encoded in their filename.  Note that a day is defined as 24
	// hours and may not exactly correspond to calendar days due to daylight
	// savings, leap seconds, etc. The default is not to remove old log files
	// based on age.
	MaxAge int `json:"max_age" yaml:"max_age"`

	// MaxBackups is the maximum number of old log files to retain.  The default
	// is to retain all old log files (though MaxAge may still cause them to get
	// deleted.)
	MaxBackups int `json:"max_backups" yaml:"max_backups"`

	// LocalTime determines if the time used for formatting the timestamps in
	// backup files is the computer's local time.  The default is to use UTC
	// time.
	LocalTime bool `json:"localtime" yaml:"localtime"`

	// Compress determines if the rotated log files should be compressed
	// using gzip. The default is not to perform compression.
	Compress bool `json:"compress" yaml:"compress"`
}

func (rf *RotateFile) SetDefaults() {
	if rf.MaxSize == 0 {
		rf.MaxSize = 100
	}
}
