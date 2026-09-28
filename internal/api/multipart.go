package api

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
)

// MultipartFileUpload describes a single-file multipart/form-data POST.
// Additional fields are ordinary string form fields and are written in sorted
// order for deterministic request shape outside the boundary value.
type MultipartFileUpload struct {
	FilePath      string
	FileFieldName string
	Filename      string
	Fields        map[string]string
	MaxBytes      int64
	Idempotency   Idempotency
}

// PostMultipart streams one multipart upload and decodes the standard Chab
// envelope. Multipart uploads are never retried here; callers that need
// recovery must persist and replay the same idempotency key themselves.
func (c *Client) PostMultipart(ctx context.Context, path string, upload MultipartFileUpload) (RawResult, error) {
	if upload.Idempotency.singleAttempt {
		one, err := c.singleAttemptClient()
		if err != nil {
			return RawResult{}, err
		}
		upload.Idempotency.singleAttempt = false
		return one.PostMultipart(ctx, path, upload)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if upload.FilePath == "" {
		return RawResult{}, &UsageError{Field: "file", Detail: "path must not be empty"}
	}
	if upload.FileFieldName == "" {
		upload.FileFieldName = "file"
	}
	if upload.MaxBytes < 1 {
		return RawResult{}, &UsageError{Field: "file", Detail: "maximum upload size must be positive"}
	}
	info, err := os.Stat(upload.FilePath)
	if err != nil {
		return RawResult{}, &UsageError{Field: "file", Detail: "could not stat upload file", Err: err}
	}
	if !info.Mode().IsRegular() {
		return RawResult{}, &UsageError{Field: "file", Detail: "must be a regular file"}
	}
	if info.Size() < 1 {
		return RawResult{}, &UsageError{Field: "file", Detail: "must not be empty"}
	}
	if info.Size() > upload.MaxBytes {
		return RawResult{}, &UsageError{Field: "file", Detail: "exceeds configured byte limit"}
	}

	key, err := resolveRequestIdempotency(http.MethodPost, upload.Idempotency)
	if err != nil {
		return RawResult{}, err
	}
	redactor := c.newRequestRedactor(key)
	target := c.requestURL(path, nil)
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	go writeMultipartUpload(pw, writer, upload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, pr)
	if err != nil {
		_ = pr.Close()
		return RawResult{}, &UsageError{Field: "path", Detail: "could not build multipart request URL", Err: redactor.redactErr(err)}
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("User-Agent", "chab/"+c.userAgentVersion)
	if c.locale != "" {
		req.Header.Set("Accept-Language", c.locale)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}

	c.debugfWith(redactor, "request POST %s attempt=1 idempotency=explicit multipart=true", target)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return RawResult{}, c.transportError(err, 1, nil, redactor)
	}
	meta := captureResponseMeta(resp.StatusCode, resp.Header, c.now())
	c.debugfWith(redactor, "response status=%d request_id=%s attempt=1", resp.StatusCode, meta.RequestID)
	c.debugMetaWith(redactor, meta)
	body, readErr := readAndClose(resp.Body)
	if readErr != nil {
		return RawResult{}, c.transportError(readErr, 1, nil, redactor)
	}
	meta.Attempts = 1
	meta.IdempotencyUsed = key != ""
	return c.decodeRaw(resp.StatusCode, body, meta, redactor)
}

func writeMultipartUpload(pipe *io.PipeWriter, writer *multipart.Writer, upload MultipartFileUpload) {
	var err error
	defer func() {
		if err != nil {
			_ = pipe.CloseWithError(err)
			return
		}
		_ = pipe.Close()
	}()
	keys := make([]string, 0, len(upload.Fields))
	for key := range upload.Fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err = writer.WriteField(key, upload.Fields[key]); err != nil {
			return
		}
	}
	filename := upload.Filename
	if filename == "" {
		filename = filepath.Base(upload.FilePath)
	}
	var part io.Writer
	part, err = writer.CreateFormFile(upload.FileFieldName, filename)
	if err != nil {
		return
	}
	var file *os.File
	file, err = os.Open(upload.FilePath)
	if err != nil {
		return
	}
	defer file.Close()
	_, err = copyBounded(part, file, upload.MaxBytes)
	if err != nil {
		return
	}
	err = writer.Close()
}

func copyBounded(dst io.Writer, src io.Reader, maxBytes int64) (int64, error) {
	var written int64
	buf := make([]byte, 32*1024)
	for {
		if written == maxBytes {
			var extra [1]byte
			n, err := src.Read(extra[:])
			if n > 0 {
				return written, fmt.Errorf("upload file exceeds configured byte limit")
			}
			if err == io.EOF {
				return written, nil
			}
			if err != nil {
				return written, err
			}
			continue
		}
		limit := int64(len(buf))
		if remaining := maxBytes - written; remaining < limit {
			limit = remaining
		}
		n, readErr := src.Read(buf[:limit])
		if n > 0 {
			m, writeErr := dst.Write(buf[:n])
			written += int64(m)
			if writeErr != nil {
				return written, writeErr
			}
			if m != n {
				return written, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}
