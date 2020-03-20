package grpc_gateway_common

import "google.golang.org/grpc"

// GRPCStreamInterceptors ...
func (s *BaseGRPCService) GRPCStreamInterceptors(interceptors ...grpc.StreamServerInterceptor) *BaseGRPCService {
	s.gRPCStreamInterceptors = interceptors
	return s
}
