package grpc_gateway_common

import "net/http"

// HTTPServerInterceptor ...
type HTTPServerInterceptor func(handler http.Handler) http.Handler

// HTTPMiddleware ...
func (s *BaseGRPCService) HTTPMiddleware(interceptors ...HTTPServerInterceptor) *BaseGRPCService {
	s.httpInterceptors = interceptors
	return s
}
