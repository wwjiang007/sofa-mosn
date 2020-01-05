/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package log

import (
	"context"

	"mosn.io/mosn/pkg/types"
)

type Level uint8

const (
	FATAL Level = iota
	ERROR
	WARN
	INFO
	DEBUG
	TRACE
	RAW
)

const (
	InfoPre  string = "[INFO]"
	DebugPre string = "[DEBUG]"
	WarnPre  string = "[WARN]"
	ErrorPre string = "[ERROR]"
	FatalPre string = "[FATAL]"
	TracePre string = "[TRACE]"
)

// ErrorLogger generates lines of output to an io.Writer
type ErrorLogger interface {
	Println(args ...interface{})

	Printf(format string, args ...interface{})

	// Alertf is a wrapper of Errorf
	Alertf(errkey types.ErrorKey, format string, args ...interface{})

	Infof(format string, args ...interface{})

	Debugf(format string, args ...interface{})

	Warnf(format string, args ...interface{})

	Errorf(format string, args ...interface{})

	Tracef(format string, args ...interface{})

	Fatalf(format string, args ...interface{})

	Fatal(args ...interface{})

	Fatalln(args ...interface{})

	// SetLogLevel updates the log level
	SetLogLevel(Level)
	// GetLogLevel returns the logger's level
	GetLogLevel() Level

	// Toggle disable/enable the logger
	Toggle(disable bool)
}

// ProxyLogger generates lines of output to an io.Writer, works for data flow
type ProxyLogger interface {
	// Alertf is a wrapper of Errorf
	Alertf(ctx context.Context, errkey types.ErrorKey, format string, args ...interface{})

	Infof(ctx context.Context, format string, args ...interface{})

	Debugf(ctx context.Context, format string, args ...interface{})

	Warnf(ctx context.Context, format string, args ...interface{})

	Errorf(ctx context.Context, format string, args ...interface{})

	Fatalf(ctx context.Context, format string, args ...interface{})

	// SetLogLevel updates the log level
	SetLogLevel(Level)
	// GetLogLevel returns the logger's level
	GetLogLevel() Level

	// Toggle disable/enable the logger
	Toggle(disable bool)
}

// CreateErrorLoggerFunc creates a ErrorLogger implementation by output and level
type CreateErrorLoggerFunc func(output string, level Level) (ErrorLogger, error)
