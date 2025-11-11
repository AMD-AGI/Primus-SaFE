package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/mapUtil"
	"io"
)

const (
	CodeSuccess int = 2000
)

var (
	successMeta = Meta{
		Code:    CodeSuccess,
		Message: "OK",
	}
)

type Meta struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Trace struct {
	TraceId string `json:"trace_id"`
	SpanId  string `json:"span_id"`
}

type Response struct {
	Meta    Meta        `json:"meta"`
	Data    interface{} `json:"data"`
	Tracing *Trace      `json:"tracing"`
}

type ListData struct {
	Rows       interface{} `json:"rows"`
	TotalCount int         `json:"total_count"`
}

func newResponse(ctx context.Context, meta Meta, data interface{}) Response {
	resp := Response{
		Meta: meta,
		Data: data,
	}
	// extract trace
	span, has := trace.SpanFromContext(ctx)
	if has {
		traceId, spanId, isJager := trace.GetTraceIDAndSpanID(span)
		if isJager {
			resp.Tracing = &Trace{
				TraceId: traceId,
				SpanId:  spanId,
			}
		}
	}
	return resp
}

func SuccessResp(ctx context.Context, data interface{}) Response {
	return newResponse(ctx, successMeta, data)
}

func ErrorResp(ctx context.Context, code int, errMsg string, data interface{}) Response {
	meta := Meta{
		Code:    code,
		Message: errMsg,
	}
	return newResponse(ctx, meta, data)
}

type Error struct {
	Code        int
	Message     string
	OriginError error
}

func (e Error) Error() string {
	return fmt.Sprintf("Code %d.Message %s.Origin error %+v", e.Code, e.Message, e.OriginError)
}

func ParseResponse(bodyReader io.Reader, targetData interface{}) (*Meta, *Trace, error) {
	buffer := &bytes.Buffer{}
	_, err := buffer.ReadFrom(bodyReader)
	if err != nil {
		return nil, nil, err
	}
	resp := &Response{}
	err = json.Unmarshal(buffer.Bytes(), resp)
	if err != nil {
		return nil, nil, err
	}
	if resp.Meta.Code == 0 {
		return nil, nil, errors.NewError().WithCode(errors.ClientError).WithMessage("Remote side returned no data")
	}
	if resp.Meta.Code != CodeSuccess {
		return &resp.Meta, resp.Tracing, errors.NewError().WithCode(resp.Meta.Code).WithMessage(resp.Meta.Message)
	}
	err = mapUtil.DecodeFromMap(resp.Data, targetData)
	if err != nil {
		return &resp.Meta, resp.Tracing, errors.NewError().WithCode(errors.ClientError).WithError(err).WithMessage("Failed to parse body")
	}
	return &resp.Meta, resp.Tracing, nil
}

func NewListData(datas interface{}, totalCount int) ListData {
	return ListData{
		Rows:       datas,
		TotalCount: totalCount,
	}
}
