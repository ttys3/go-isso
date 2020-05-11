package server

import (
	"compress/flate"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"strings"

	"wrong.wang/x/go-isso/logger"
)

const compressionThreshold = 128

// Builder generates HTTP responses.
type builder struct {
	w                 http.ResponseWriter
	r                 *http.Request
	err               error
	statusCode        int
	headers           map[string]string
	enableCompression bool
	body              interface{}
	written           bool
}

// WithStatus uses the given status code to build the response.
func (b *builder) WithStatus(statusCode int) {
	b.statusCode = statusCode
}

// WithError save the error happend during response
func (b *builder) WithError(err error) {
	b.err = err
}

// WithHeader adds the given HTTP header to the response.
func (b *builder) WithHeader(key, value string) {
	b.headers[key] = value
}

// WithBody uses the given body to build the response.
func (b *builder) WithBody(body interface{}) {
	b.body = body
}

func (b *builder) writeHeaders() {
	for key, value := range b.headers {
		b.w.Header().Set(key, value)
	}

	b.w.WriteHeader(b.statusCode)
}

func (b *builder) compress(data []byte) {
	if b.enableCompression && len(data) > compressionThreshold {
		acceptEncoding := b.r.Header.Get("Accept-Encoding")

		switch {
		case strings.Contains(acceptEncoding, "gzip"):
			b.headers["Content-Encoding"] = "gzip"
			b.writeHeaders()

			gzipWriter := gzip.NewWriter(b.w)
			defer gzipWriter.Close()
			gzipWriter.Write(data)
			return
		case strings.Contains(acceptEncoding, "deflate"):
			b.headers["Content-Encoding"] = "deflate"
			b.writeHeaders()

			flateWriter, _ := flate.NewWriter(b.w, -1)
			defer flateWriter.Close()
			flateWriter.Write(data)
			return
		}
	}

	b.writeHeaders()
	b.w.Write(data)
}

// Write generates the HTTP response.
func (b *builder) Write() {
	if b.written == true {
		return
	}
	b.written = true
	if b.body == nil {
		b.writeHeaders()
		return
	}

	if b.statusCode >= 400 || b.err != nil {
		if b.err != nil {
			err := errors.Unwrap(b.err)
			if err == nil {
				err = b.err
			}
			logger.Error("%d %s => %v", b.statusCode, b.r.URL, err)
		} else {
			logger.Error("%d %s", b.statusCode, b.r.URL)
		}
	}

	switch v := b.body.(type) {
	case []byte:
		b.compress(v)
	case string:
		b.compress([]byte(v))
	case error:
		b.compress([]byte(v.Error()))
	case io.Reader:
		// Compression not implemented in this case
		b.writeHeaders()
		io.Copy(b.w, v)
	}
}

// newBuilder creates a new response builder.
func newBuilder(w http.ResponseWriter, r *http.Request) *builder {
	return &builder{w: w, r: r, statusCode: http.StatusOK, headers: make(map[string]string), enableCompression: true}
}
