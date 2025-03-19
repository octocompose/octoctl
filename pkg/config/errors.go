package config

import "errors"

var (
	// ErrNotExistent happens when a config key is not existent.
	ErrNotExistent = errors.New("no such config key")

	// ErrTypesDontMatch happens when types don't match during Get[T]().
	ErrTypesDontMatch = errors.New("config key requested type and actual type don't match")

	// ErrUnknownScheme happens when you didn't import the plugin for the scheme or the scheme is unknown.
	ErrUnknownScheme = errors.New("unknown config source scheme")

	// ErrFileNotFound happens when theres no file.
	ErrFileNotFound = errors.New("file not found")

	// ErrCodecNotFound happens when the required codec is not found.
	ErrCodecNotFound = errors.New("marshaler for codec not found")
)
