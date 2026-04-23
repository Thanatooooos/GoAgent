package convention

type Result[T any] struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"`
	Data      T      `json:"data"`
}

func (r *Result[T]) SetData(data T) {
	r.Data = data
}
