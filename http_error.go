package grpc_gateway_common

import (
	"context"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/status"
	"io"
	"net/http"
)

// TransformErrors transform function errors to HTTP errors
func TransformErrors(ctx context.Context, mux *runtime.ServeMux, marshaler runtime.Marshaler, w http.ResponseWriter, r *http.Request, err error) {
	const fallback = `{"error":{"code":-1,"message":"failed to marshal error message"}}`

	s, _ := status.FromError(err)

	w.Header().Del("Trailer")

	contentType := marshaler.ContentType()
	// Check marshaler on run time in order to keep backwards compatability
	// An interface param needs to be added to the ContentType() function on
	// the Marshal interface to be able to remove this check
	if httpBodyMarshaler, ok := marshaler.(*runtime.HTTPBodyMarshaler); ok {
		pb := s.Proto()
		contentType = httpBodyMarshaler.ContentTypeFromMessage(pb)
	}
	w.Header().Set("Content-Type", contentType)

	w.WriteHeader(http.StatusInternalServerError)
	if _, err := io.WriteString(w, fallback); err != nil {
		grpclog.Infof("Failed to write response: %v", err)
	}
	return
}
