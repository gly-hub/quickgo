package grpcep

type JsonResponse struct {
	Code       int32         `json:"code"`
	Msg        string      `json:"msg"`
	Data       interface{} `json:"data"`
	HttpStatus int         `json:"-"`
	RequestId  string      `json:"request_id"`
}
