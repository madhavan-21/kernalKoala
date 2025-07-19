package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// LogLevel represents different log levels
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

// String returns string representation of log level
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Logger represents our custom logger
type Logger struct {
	logger    *log.Logger
	level     LogLevel
	file      *os.File
	mu        sync.Mutex
	colorized bool
}

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[37m"
)

var (
	defaultLogger *Logger
	once          sync.Once
)

// Config holds logger configuration
type Config struct {
	Level     LogLevel
	AddCaller bool
	AddTime   bool
	FilePath  string
}

// DefaultConfig returns default logger configuration
func DefaultConfig() *Config {
	return &Config{
		Level:     ERROR,
		AddCaller: true,
		AddTime:   true,
		FilePath:  os.Getenv("LOG_PATH"),
	}
}

// NewLogger creates a new logger instance
func NewLogger(config *Config) (*Logger, error) {
	if config == nil {
		config = DefaultConfig()
	}

	var writer io.Writer
	var file *os.File
	var colorized bool

	// Determine output destination based on LOG_PATH environment variable
	if config.FilePath != "" {
		// Ensure directory exists
		if err := os.MkdirAll(filepath.Dir(config.FilePath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}

		// Open log file with append mode
		f, err := os.OpenFile(config.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		file = f
		writer = f
		colorized = false
	} else {
		writer = os.Stdout
		colorized = true // Enable colors for stdout
	}

	// Create logger with custom flags
	flags := 0
	if config.AddTime {
		flags |= log.LstdFlags
	}

	logger := &Logger{
		logger:    log.New(writer, "", flags),
		level:     config.Level,
		file:      file,
		colorized: colorized,
	}

	return logger, nil
}

// GetDefaultLogger returns the default logger instance (singleton)
func GetDefaultLogger() *Logger {
	once.Do(func() {
		var err error
		defaultLogger, err = NewLogger(DefaultConfig())
		if err != nil {
			// Fallback to stdout if file creation fails
			config := DefaultConfig()
			config.FilePath = ""
			defaultLogger, _ = NewLogger(config)
		}
	})
	return defaultLogger
}

// Close closes the log file if open
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// SetLevel sets the minimum log level
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// isLevelEnabled checks if given level should be logged
func (l *Logger) isLevelEnabled(level LogLevel) bool {
	return level >= l.level
}

// getColor returns ANSI color code for log level
func (l *Logger) getColor(level LogLevel) string {
	if !l.colorized {
		return ""
	}

	switch level {
	case DEBUG:
		return colorGray
	case INFO:
		return colorCyan
	case WARN:
		return colorYellow
	case ERROR:
		return colorRed
	case FATAL:
		return colorPurple
	default:
		return colorReset
	}
}

// formatMessage formats the log message with level, caller info, and color
func (l *Logger) formatMessage(level LogLevel, msg string, addCaller bool) string {
	color := l.getColor(level)
	reset := ""
	if l.colorized {
		reset = colorReset
	}

	levelStr := fmt.Sprintf("[%s]", level.String())

	if addCaller {
		_, file, line, ok := runtime.Caller(3)
		if ok {
			filename := filepath.Base(file)
			callerInfo := fmt.Sprintf(" %s:%d", filename, line)
			return fmt.Sprintf("%s%s%s%s %s", color, levelStr, callerInfo, reset, msg)
		}
	}

	return fmt.Sprintf("%s%s%s %s", color, levelStr, reset, msg)
}

// log is the main logging function
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if !l.isLevelEnabled(level) {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	msg := fmt.Sprintf(format, args...)
	formattedMsg := l.formatMessage(level, msg, true)

	l.logger.Print(formattedMsg)

	// Auto-exit for FATAL level
	if level == FATAL {
		os.Exit(1)
	}
}

// Debug logs debug level messages
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

// Info logs info level messages
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

// Warn logs warning level messages
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

// Error logs error level messages
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

// Fatal logs fatal level messages and exits
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(FATAL, format, args...)
}

// Package-level convenience functions using default logger

// Debug logs debug message using default logger
func Debug(format string, args ...interface{}) {
	GetDefaultLogger().Debug(format, args...)
}

// Info logs info message using default logger
func Info(format string, args ...interface{}) {
	GetDefaultLogger().Info(format, args...)
}

// Warn logs warning message using default logger
func Warn(format string, args ...interface{}) {
	GetDefaultLogger().Warn(format, args...)
}

// Error logs error message using default logger
func Error(format string, args ...interface{}) {
	GetDefaultLogger().Error(format, args...)
}

// Fatal logs fatal message using default logger and exits
func Fatal(format string, args ...interface{}) {
	GetDefaultLogger().Fatal(format, args...)
}

// SetLevel sets log level for default logger
func SetLevel(level LogLevel) {
	GetDefaultLogger().SetLevel(level)
}

// Close closes the default logger
func Close() error {
	return GetDefaultLogger().Close()
}

// Example usage and testing
func ExampleUsage() {
	// Using default logger (respects LOG_PATH env var)
	Info("Application starting...")
	Debug("Debug message - won't show unless level is DEBUG")
	Warn("This is a warning")
	Error("This is an error")

	// Using custom logger
	config := &Config{
		Level:     DEBUG,
		AddCaller: true,
		AddTime:   true,
		FilePath:  "/tmp/custom.log",
	}

	customLogger, err := NewLogger(config)
	if err != nil {
		Fatal("Failed to create custom logger: %v", err)
	}
	defer customLogger.Close()

	customLogger.Info("Custom logger message")
	customLogger.Debug("Debug from custom logger")

	// Change log level dynamically
	SetLevel(WARN)
	Info("This won't show")
	Warn("This will show")
}
