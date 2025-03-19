// Package codecs contains various text codecs.
package codecs

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// Codec is able to encode/decode a content type to/from a byte sequence.
type Codec struct {
	Mime string
}

// Marshal encodes any pointer into json byte.
func (c *Codec) Marshal(v any) ([]byte, error) {
	switch c.Mime {
	case MimeJSON:
		return json.Marshal(v)
	case MimeYAML:
		return yaml.Marshal(v)
	case MimeTOML:
		return toml.Marshal(v)
	default:
		return nil, fmt.Errorf("unknown mime type: %s", c.Mime)
	}
}

// Unmarshal decodes json bytes into object v.
// Param v should be a pointer type.
func (c *Codec) Unmarshal(data []byte, v any) error {
	switch c.Mime {
	case MimeJSON:
		return json.Unmarshal(data, v)
	case MimeYAML:
		return yaml.Unmarshal(data, v)
	case MimeTOML:
		return toml.Unmarshal(data, v)
	default:
		return fmt.Errorf("unknown mime type: %s", c.Mime)
	}
}

// NewEncoder returns an Encoder which writes bytes sequence into "w".
func (c *Codec) NewEncoder(w io.Writer) Encoder {
	switch c.Mime {
	case MimeJSON:
		encoder := json.NewEncoder(w)

		return EncoderFunc(encoder.Encode)
	case MimeYAML:
		encoder := yaml.NewEncoder(w)

		return EncoderFunc(encoder.Encode)
	case MimeTOML:
		encoder := toml.NewEncoder(w)

		return EncoderFunc(encoder.Encode)
	default:
		return EncoderFunc(func(any) error { return fmt.Errorf("unknown mime type: %s", c.Mime) })
	}
}

// NewDecoder returns a Decoder which reads byte sequence from "r".
func (c *Codec) NewDecoder(r io.Reader) Decoder {
	switch c.Mime {
	case MimeJSON:
		decoder := json.NewDecoder(r)

		return DecoderFunc(decoder.Decode)
	case MimeYAML:
		decoder := yaml.NewDecoder(r)

		return DecoderFunc(decoder.Decode)
	case MimeTOML:
		decoder := toml.NewDecoder(r)

		return DecoderFunc(func(v any) error {
			_, err := decoder.Decode(v)

			return err
		})
	default:
		return DecoderFunc(func(any) error { return fmt.Errorf("unknown mime type: %s", c.Mime) })
	}
}

// Decoder decodes a byte sequence.
type Decoder interface {
	Decode(v any) error
}

// Encoder encodes payloads / fields into byte sequence.
type Encoder interface {
	Encode(v any) error
}

// DecoderFunc adapts an decoder function into Decoder.
type DecoderFunc func(v any) error

// Decode delegates invocations to the underlying function itself.
func (f DecoderFunc) Decode(v any) error { return f(v) }

// EncoderFunc adapts an encoder function into Encoder.
type EncoderFunc func(v any) error

// Encode delegates invocations to the underlying function itself.
func (f EncoderFunc) Encode(v any) error { return f(v) }

// mimeMap is a map of MIME types to their associated codecs.
//
//nolint:gochecknoglobals
var mimeMap = map[string]Codec{
	MimeJSON: Codec{Mime: MimeJSON},
	MimeYAML: Codec{Mime: MimeYAML},
	MimeTOML: Codec{Mime: MimeTOML},
}

// extensionMap is a map of file extensions to their associated MIME types.
//
//nolint:gochecknoglobals
var extensionMap = map[string]string{
	".json": MimeJSON,
	".yml":  MimeYAML,
	".yaml": MimeYAML,
	".toml": MimeTOML,
}

// formatMap is a map of formats to their associated MIME types.
//
//nolint:gochecknoglobals
var formatMap = map[string]string{
	"json": MimeJSON,
	"yaml": MimeYAML,
	"toml": MimeTOML,
}

// GetMime returns the codec associated with the given MIME type.
func GetMime(mimeType string) (Codec, error) {
	if codec, ok := mimeMap[mimeType]; ok {
		return codec, nil
	}

	return Codec{}, fmt.Errorf("unknown mime type: %s", mimeType)
}

// GetExtension returns the codec associated with the given file extension.
func GetExtension(extension string) (Codec, error) {
	if mimeType, ok := extensionMap[extension]; ok {
		return mimeMap[mimeType], nil
	}

	return Codec{}, fmt.Errorf("unknown file extension: %s", extension)
}

// GetFormat returns the codec associated with the given format.
func GetFormat(format string) (Codec, error) {
	if mimeType, ok := formatMap[format]; ok {
		return mimeMap[mimeType], nil
	}

	return Codec{}, fmt.Errorf("unknown format: %s", format)
}
