package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const maxOCRImageBytes = 10 << 20

type OCRResult struct {
	Status string `json:"status"`
	Text   string `json:"text"`
}

type OCRError struct {
	Code       string
	HTTPStatus int
	Permanent  bool
}

func (e *OCRError) Error() string {
	if e.HTTPStatus != 0 {
		return fmt.Sprintf("OCR service HTTP %d", e.HTTPStatus)
	}
	return "OCR " + e.Code
}

func ClassifyOCRError(err error) (string, bool) {
	var ocrError *OCRError
	if errors.As(err, &ocrError) {
		return ocrError.Code, ocrError.Permanent
	}
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
		return "ocr_timeout", false
	}
	return "ocr_request", false
}

type OCRClient interface {
	Recognize(ctx context.Context, image []byte) (OCRResult, error)
}

type HTTPOCRClient struct {
	url    string
	client *http.Client
}

func NewHTTPOCRClient(url string, timeout time.Duration) *HTTPOCRClient {
	if timeout <= 0 {
		timeout = 35 * time.Second
	}
	return &HTTPOCRClient{
		url:    strings.TrimRight(strings.TrimSpace(url), "/") + "/recognize",
		client: &http.Client{Timeout: timeout},
	}
}

func (c *HTTPOCRClient) Recognize(ctx context.Context, image []byte) (OCRResult, error) {
	if len(image) == 0 || len(image) > maxOCRImageBytes {
		return OCRResult{}, &OCRError{Code: "ocr_image_size", Permanent: true}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(image))
	if err != nil {
		return OCRResult{}, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.client.Do(req)
	if err != nil {
		return OCRResult{}, fmt.Errorf("call OCR service: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		code, permanent := "ocr_provider", false
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			code, permanent = "ocr_auth", true
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			code, permanent = "ocr_request_invalid", true
		case http.StatusTooManyRequests:
			code = "ocr_rate_limit"
		}
		return OCRResult{}, &OCRError{Code: code, HTTPStatus: resp.StatusCode, Permanent: permanent}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return OCRResult{}, &OCRError{Code: "ocr_response_invalid"}
	}
	var result OCRResult
	if err := json.Unmarshal(data, &result); err != nil {
		return OCRResult{}, &OCRError{Code: "ocr_response_invalid"}
	}
	if result.Status != "success" && result.Status != "no_text" {
		return OCRResult{}, &OCRError{Code: "ocr_response_invalid"}
	}
	result.Text = strings.TrimSpace(result.Text)
	if result.Status == "no_text" && result.Text != "" {
		return OCRResult{}, &OCRError{Code: "ocr_response_invalid"}
	}
	if result.Status == "success" && result.Text == "" {
		return OCRResult{}, &OCRError{Code: "ocr_response_invalid"}
	}
	return result, nil
}
