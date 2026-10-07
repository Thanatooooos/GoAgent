package parser_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	parser "local/rag-project/internal/app/core/parser"
	docreaderpb "local/rag-project/internal/app/core/parser/docreaderpb"
)

type testDocReader struct {
	docreaderpb.UnimplementedDocReaderServer
	request *docreaderpb.ReadRequest
}

type testOCR struct{}

func (testOCR) Recognize(context.Context, []byte) (parser.OCRResult, error) {
	return parser.OCRResult{Status: "success", Text: "chart says 42"}, nil
}

type failingOCR struct{}

func (failingOCR) Recognize(context.Context, []byte) (parser.OCRResult, error) {
	return parser.OCRResult{}, errors.New("OCR unavailable")
}

func (s *testDocReader) ReadStream(request *docreaderpb.ReadRequest, stream docreaderpb.DocReader_ReadStreamServer) error {
	s.request = request
	if err := stream.Send(&docreaderpb.ReadStreamResponse{Payload: &docreaderpb.ReadStreamResponse_Meta{Meta: &docreaderpb.ReadStreamMeta{
		MarkdownContent: "# 标题\n\n正文\n\n![](images/chart.png)",
		Metadata:        map[string]string{"pages": "2"},
	}}}); err != nil {
		return err
	}
	return stream.Send(&docreaderpb.ReadStreamResponse{Payload: &docreaderpb.ReadStreamResponse_Image{Image: &docreaderpb.ImageRef{Filename: "chart.png", OriginalRef: "images/chart.png"}}})
}

func TestDocReaderParsesThroughStream(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	stub := &testDocReader{}
	docreaderpb.RegisterDocReaderServer(server, stub)
	go server.Serve(listener)
	defer server.Stop()

	docParser, err := parser.NewDocReaderDocumentParser(listener.Addr().String(), 5*time.Second, nil, testOCR{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := docParser.Parse(context.Background(), []byte("file bytes"), "application/pdf", map[string]any{"file_name": "report.pdf"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "# 标题\n\n正文\n\nchart says 42" {
		t.Fatalf("unexpected text: %q", result.Text)
	}
	if result.Metadata["pages"] != "2" || result.Metadata["parser_type"] != parser.ParserTypeDocReader {
		t.Fatalf("unexpected metadata: %#v", result.Metadata)
	}
	if stub.request.GetFileName() != "report.pdf" || stub.request.GetFileType() != "pdf" || stub.request.GetConfig().GetParserEngine() != "builtin" {
		t.Fatalf("unexpected request: %#v", stub.request)
	}
}

func TestDocReaderStructuredKeepsImageSeparateFromText(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	docreaderpb.RegisterDocReaderServer(server, &testDocReader{})
	go server.Serve(listener)
	defer server.Stop()

	docParser, err := parser.NewDocReaderDocumentParser(listener.Addr().String(), 5*time.Second, nil, failingOCR{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := docParser.ParseStructured(context.Background(), []byte("file bytes"), "application/pdf", map[string]any{"file_name": "report.pdf"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Text, "chart says 42") || len(result.Images) != 1 || result.Images[0].OriginalRef != "images/chart.png" {
		t.Fatalf("unexpected structured result: %#v", result)
	}
	if result.Images[0].Error != "image frame has no data" || !result.ImageInventoryConfirmed {
		t.Fatalf("missing image provenance: %#v", result.Images[0])
	}
}

func TestDocReaderFallsBackToTika(t *testing.T) {
	tikaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fallback text"))
	}))
	defer tikaServer.Close()

	docParser, err := parser.NewDocReaderDocumentParser("127.0.0.1:1", time.Second,
		parser.NewTikaDocumentParser(tikaServer.Client(), tikaServer.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := docParser.Parse(context.Background(), []byte("file bytes"), "application/pdf", map[string]any{"file_name": "report.pdf"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "fallback text" || result.Metadata["fallback_from"] != parser.ParserTypeDocReader {
		t.Fatalf("unexpected fallback result: %#v", result)
	}
}

func TestDocReaderDoesNotFallbackAfterOCRFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	docreaderpb.RegisterDocReaderServer(server, &testDocReader{})
	go server.Serve(listener)
	defer server.Stop()
	fallbackCalled := false
	tikaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalled = true
		_, _ = w.Write([]byte("fallback text"))
	}))
	defer tikaServer.Close()
	docParser, err := parser.NewDocReaderDocumentParser(listener.Addr().String(), 5*time.Second,
		parser.NewTikaDocumentParser(tikaServer.Client(), tikaServer.URL), failingOCR{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = docParser.Parse(context.Background(), []byte("file bytes"), "application/pdf", map[string]any{"file_name": "report.pdf"})
	if err == nil || !strings.Contains(err.Error(), "OCR unavailable") || fallbackCalled {
		t.Fatalf("expected OCR failure without fallback, got err=%v fallback=%t", err, fallbackCalled)
	}
}
