package octoconfig

import (
	"errors"
	"net/url"
	"strings"
)

// JURL represents a URL in JSON.
type JURL struct {
	// URL is the underlying URL.
	*url.URL
}

// String returns the string representation of the JSONURL.
func (j *JURL) String() string {
	if j == nil || j.URL == nil {
		return ""
	}

	return j.URL.String()
}

// Copy returns a copy of the JURL.
func (j *JURL) Copy() (*JURL, error) {
	if j == nil || j.URL == nil {
		return nil, errors.New("JURL is nil")
	}

	return NewJURL(j.URL.String())
}

// NewJURL creates a new `json` URL.
func NewJURL(s string) (*JURL, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}

	return &JURL{URL: u}, nil
}

// UnmarshalJSON unmarshals the JURL from JSON.
func (j *JURL) UnmarshalJSON(data []byte) error {
	u, err := url.Parse(strings.Trim(string(data), `"`))
	if err != nil {
		return err
	}

	j.URL = u

	return nil
}

// MarshalJSON marshals the JURL to JSON.
func (j *JURL) MarshalJSON() ([]byte, error) {
	if j == nil || j.URL == nil {
		return nil, nil
	}

	return []byte(`"` + j.URL.String() + `"`), nil
}
