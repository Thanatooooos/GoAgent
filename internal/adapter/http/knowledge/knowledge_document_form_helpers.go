package knowledge

import (
	"mime/multipart"
	"strings"
)

func parseBool(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "1" || value == "true" || value == "yes"
}

func multipartFileSize(file *multipart.FileHeader) int64 {
	if file == nil {
		return 0
	}
	return file.Size
}

func contentTypeFromHeader(file *multipart.FileHeader) string {
	if file == nil {
		return ""
	}
	return file.Header.Get("Content-Type")
}

func fileNameFromHeader(file *multipart.FileHeader) string {
	if file == nil {
		return ""
	}
	return file.Filename
}

func openMultipartFile(file *multipart.FileHeader) (multipart.File, func(), error) {
	if file == nil {
		return nil, func() {}, nil
	}
	opened, err := file.Open()
	if err != nil {
		return nil, func() {}, err
	}
	return opened, func() { _ = opened.Close() }, nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
