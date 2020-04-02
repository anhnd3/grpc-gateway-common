package grpc_gateway_common

import (
	"context"
	"net/http"
	"strconv"

	"github.com/golang/protobuf/proto"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const httpCodeHeader = "X-Http-Code"

// FormatHTTPResponse support sync cookie to response
func FormatHTTPResponse(ctx context.Context, w http.ResponseWriter, resp proto.Message) error {
	md, ok := runtime.ServerMetadataFromContext(ctx)
	if !ok {
		return nil
	}
	if vals := md.HeaderMD.Get(httpCodeHeader); len(vals) > 0 {
		code, err := strconv.Atoi(vals[0])
		if err != nil {
			return err
		}
		w.WriteHeader(code)
	}
	return nil
}

// SetHTTPCodeHeader ...
func SetHTTPCodeHeader(ctx context.Context, code string) {
	md := metadata.Pairs(httpCodeHeader, code)
	grpc.SetHeader(ctx, md)
}
