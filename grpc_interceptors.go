package grpc_gateway_common

import "google.golang.org/grpc"

// GRPCUnaryInterceptors ...
func (s *BaseGRPCService) GRPCUnaryInterceptors(interceptors ...grpc.UnaryServerInterceptor) *BaseGRPCService {
	s.gRPCUnaryInterceptors = interceptors
	return s
}
