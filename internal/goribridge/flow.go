// Package goribridge delivers completed Gori captures to JXScout without fetching
// target URLs itself. Its input contract is Gori v0.9.1's CLI JSON export.
package goribridge

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Row struct {
	ID          int64  `json:"id"`
	URL         string `json:"url"`
	Method      string `json:"method"`
	Status      int    `json:"status"`
	State       string `json:"state"`
	ContentType string `json:"content_type"`
}

type Body struct {
	Encoding        string `json:"encoding"`
	Text            string `json:"text"`
	Base64          string `json:"base64"`
	Size            int64  `json:"size"`
	Truncated       bool   `json:"truncated"`
	WireTruncated   bool   `json:"wire_truncated"`
	DecodeTruncated bool   `json:"decode_truncated"`
	Omitted         bool   `json:"omitted"`
	Note            string `json:"note"`
}

type Message struct {
	Head       string `json:"head"`
	HeadBase64 string `json:"head_base64"`
	HeadLossy  bool   `json:"head_lossy"`
	Body       *Body  `json:"body"`
}

type Flow struct {
	Flow     Row     `json:"flow"`
	Request  Message `json:"request"`
	Response Message `json:"response"`
	Error    string  `json:"error"`
}

// Ingestion matches the existing jxscout-burp/Caido adapter protocol.
type Ingestion struct {
	RequestURL string `json:"requestUrl"`
	Request    string `json:"request"`
	Response   string `json:"response"`
}

type Scope struct {
	Origins   []string `json:"origins"`
	Methods   []string `json:"methods"`
	ExpiresAt *string  `json:"expiresAt"`
}

func origin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	host := strings.ToLower(u.Host)
	if u.Scheme == "http" && u.Port() == "80" || u.Scheme == "https" && u.Port() == "443" {
		host = strings.TrimSuffix(host, ":"+u.Port())
	}
	return strings.ToLower(u.Scheme) + "://" + host
}

func (s *Scope) Active(now time.Time) bool {
	if s == nil || len(s.Origins) == 0 {
		return false
	}
	if s.ExpiresAt == nil {
		return true
	}
	expires, err := time.Parse(time.RFC3339Nano, *s.ExpiresAt)
	return err == nil && now.Before(expires)
}

func (s *Scope) Allows(row Row) bool {
	if !s.Active(time.Now()) {
		return false
	}
	allowed := false
	for _, item := range s.Origins {
		if origin(item) != "" && origin(item) == origin(row.URL) {
			allowed = true
			break
		}
	}
	if !allowed {
		return false
	}
	for _, method := range s.Methods {
		if strings.EqualFold(method, row.Method) {
			return true
		}
	}
	return false
}

func (s *Scope) fingerprint() string {
	data, _ := json.Marshal(s)
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func bodyBytes(body *Body) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	if body.Truncated || body.WireTruncated || body.DecodeTruncated || body.Omitted {
		return nil, fmt.Errorf("incomplete_body")
	}
	var data []byte
	switch body.Encoding {
	case "text":
		data = []byte(body.Text)
	case "base64":
		var err error
		data, err = base64.StdEncoding.DecodeString(body.Base64)
		if err != nil {
			return nil, fmt.Errorf("invalid_base64")
		}
	default:
		return nil, fmt.Errorf("unknown_body_encoding")
	}
	if int64(len(data)) != body.Size {
		return nil, fmt.Errorf("body_size_mismatch")
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("non_utf8_body")
	}
	return data, nil
}

// Gori's body object is content-decoded. Reconstruct a consistent HTTP message,
// rather than pairing inflated bytes with gzip/chunked wire headers.
func decodedMessage(message Message, body []byte) (string, http.Header, error) {
	head := message.Head
	if message.HeadLossy {
		b, err := base64.StdEncoding.DecodeString(message.HeadBase64)
		if err != nil || !utf8.Valid(b) {
			return "", nil, fmt.Errorf("invalid_head")
		}
		head = string(b)
	}
	lines := strings.Split(strings.ReplaceAll(head, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return "", nil, fmt.Errorf("missing_head")
	}
	clean := []string{lines[0]}
	headers := http.Header{}
	for _, line := range lines[1:] {
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok || name == "" {
			return "", nil, fmt.Errorf("invalid_header")
		}
		key := http.CanonicalHeaderKey(name)
		if key == "Content-Encoding" && message.Body != nil && (!strings.Contains(message.Body.Note, "decoded:") || strings.Contains(message.Body.Note, "compressed:") || strings.Contains(message.Body.Note, "stopped after")) {
			return "", nil, fmt.Errorf("undecoded_content_encoding")
		}
		if key == "Content-Length" || key == "Content-Encoding" || key == "Transfer-Encoding" || key == "Trailer" {
			continue
		}
		headers.Add(key, strings.TrimSpace(value))
		clean = append(clean, line)
	}
	clean = append(clean, "Content-Length: "+strconv.Itoa(len(body)))
	return strings.Join(clean, "\r\n") + "\r\n\r\n" + string(body), headers, nil
}

func Eligible(row Row) bool {
	if row.Status < 200 || row.Status >= 300 {
		return false
	}
	u, err := url.Parse(row.URL)
	if err != nil {
		return false
	}
	ct := strings.ToLower(strings.Split(row.ContentType, ";")[0])
	p := strings.ToLower(u.Path)
	return ct == "text/html" || ct == "application/xhtml+xml" || strings.Contains(ct, "javascript") || strings.Contains(ct, "ecmascript") || strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".mjs")
}

func MapFlow(flow Flow) (Ingestion, string, error) {
	if flow.Error != "" {
		return Ingestion{}, "", fmt.Errorf("capture_error")
	}
	responseBody, err := bodyBytes(flow.Response.Body)
	if err != nil {
		return Ingestion{}, "", err
	}
	if flow.Response.Body == nil {
		return Ingestion{}, "", fmt.Errorf("missing_response_body")
	}
	requestBody, err := bodyBytes(flow.Request.Body)
	if err != nil {
		return Ingestion{}, "", err
	}
	request, _, err := decodedMessage(flow.Request, requestBody)
	if err != nil {
		return Ingestion{}, "", err
	}
	response, _, err := decodedMessage(flow.Response, responseBody)
	if err != nil {
		return Ingestion{}, "", err
	}
	if origin(flow.Flow.URL) == "" {
		return Ingestion{}, "", fmt.Errorf("invalid_url")
	}
	// URL plus decoded content prevents recursive recapture while allowing changed
	// assets and the same content at different URLs to retain their provenance.
	h := sha256.New()
	h.Write([]byte(flow.Flow.URL))
	h.Write([]byte{0})
	h.Write(responseBody)
	return Ingestion{flow.Flow.URL, request, response}, hex.EncodeToString(h.Sum(nil)), nil
}
