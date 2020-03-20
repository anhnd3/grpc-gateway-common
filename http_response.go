package grpc_gateway_common

import (
	"context"
	"github.com/golang/protobuf/proto"
	"net/http"
)

// FormatHTTPResponse support sync cookie to response
func FormatHTTPResponse(ctx context.Context, w http.ResponseWriter, resp proto.Message) error {
	// md, _ := runtime.ServerMetadataFromContext(ctx)
	return nil
}
