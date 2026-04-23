package test

import (
	"testing"

	aihttp "local/rag-project/internal/infra-ai/http"
)

func TestFromHttpStatus(t *testing.T) {
	if aihttp.FromHttpStatus(401) != aihttp.ErrorTypeUnauthorized {
		t.Fatalf("401 should be Unauthorized")
	}
	if aihttp.FromHttpStatus(429) != aihttp.ErrorTypeRateLimited {
		t.Fatalf("429 should be RateLimited")
	}
	if aihttp.FromHttpStatus(500) != aihttp.ErrorTypeServerError {
		t.Fatalf("500 should be ServerError")
	}
	if aihttp.FromHttpStatus(400) == aihttp.ErrorTypeServerError {
		t.Fatalf("400 should not be ServerError")
	}
}

func TestModelClientException_Is_As_Error(t *testing.T) {
	ex := aihttp.NewModelClientException("msg", aihttp.ErrorTypeClientError, 400, nil)
	if ex := ex.Error(); ex == "" {
		t.Fatalf("Error() should return non-empty string")
	}
	if !aihttp.IsModelClientException(ex) {
		t.Fatalf("IsModelClientException should return true")
	}
	if e, ok := aihttp.AsModelClientException(ex); !ok || e == nil {
		t.Fatalf("AsModelClientException should return the exception")
	}
}
