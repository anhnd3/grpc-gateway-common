package grpc_gateway_common

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/golang/protobuf/proto"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/soheilhy/cmux"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	gRPCMiddleware "github.com/grpc-ecosystem/go-grpc-middleware"
	gRPCRecovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	gRPCCtxTags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
)

// ShutdownHook ...
type ShutdownHook func()

// BaseGRPCService is a httpServer wrapper that support AddShutdownHook
type BaseGRPCService struct {
	name                   string
	port                   int
	listener               net.Listener
	gRPCRegister           GRPCRegister
	httpRegister           HTTPRegister
	hooks                  []ShutdownHook
	rootPath               string
	done                   chan error
	httpInterceptors       []HTTPServerInterceptor
	gRPCUnaryInterceptors  []grpc.UnaryServerInterceptor
	gRPCStreamInterceptors []grpc.StreamServerInterceptor
	forwardRespFunc        func(context.Context, http.ResponseWriter, proto.Message) error
}

// GRPCRegister ...
type GRPCRegister func(server *grpc.Server)

// HTTPRegister ...
type HTTPRegister func(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) (err error)

// NewGRPCServer return new service from handler, support consul
// name - service name, used to register with consul
// port - service port
func NewGRPCServer(name string, port int, gRPCRegister GRPCRegister) *BaseGRPCService {
	return &BaseGRPCService{
		name:            name,
		port:            port,
		gRPCRegister:    gRPCRegister,
		hooks:           []ShutdownHook{},
		forwardRespFunc: FormatHTTPResponse}
}

// WithResponseFunc ...
func (s *BaseGRPCService) WithResponseFunc(fn func(context.Context, http.ResponseWriter, proto.Message) error) {
	s.forwardRespFunc = fn
}

// EnableHTTP ...
func (s *BaseGRPCService) EnableHTTP(httpRegister HTTPRegister, rootPath string) *BaseGRPCService {
	s.rootPath = rootPath
	s.httpRegister = httpRegister
	return s
}

// Run listen and serve service
func (s *BaseGRPCService) Run(port int) error {
	s.port = port
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return err
	}
	s.listener = lis

	sigChan := make(chan os.Signal, 1)
	s.done = make(chan error, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go s.serve()
	fmt.Println("Server now listening")

	go func() {
		sig := <-sigChan
		fmt.Println()
		fmt.Println(sig)
		defer s.runHook()
		s.done <- s.shutdown()
	}()

	fmt.Println("Ctrl-C to interrupt...")
	err = <-s.done
	fmt.Println("Exiting...")
	return err
}

func (s *BaseGRPCService) addShutdownHook(fn ShutdownHook) {
	s.hooks = append(s.hooks, fn)
}

func (s *BaseGRPCService) runHook() {
	for _, hook := range s.hooks {
		hook()
	}
}

// Shutdown ...
func (s *BaseGRPCService) shutdown() error {
	if s.listener != nil {
		err := s.listener.Close()
		s.listener = nil
		if err != nil {
			return err
		}
	}
	fmt.Println("Shutting down server")
	return nil
}

func (s *BaseGRPCService) serve() {
	if s.httpRegister != nil {
		m := cmux.New(s.listener)
		gRPCListener := m.MatchWithWriters(cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"))
		httpListener := m.Match(cmux.HTTP1Fast())

		g := new(errgroup.Group)
		g.Go(func() error { return s.gRPCServe(gRPCListener) })
		g.Go(func() error { return s.httpServe(httpListener) })
		g.Go(func() error { return m.Serve() })

		g.Wait()
	} else {
		s.gRPCServe(s.listener)
	}
}

func (s *BaseGRPCService) gRPCServe(l net.Listener) error {
	// alwaysLoggingDeciderServer := func(ctx context.Context, fullMethodName string, servingObject interface{}) bool { return true }

	sIntOpt := grpc.StreamInterceptor(gRPCMiddleware.ChainStreamServer(
		gRPCCtxTags.StreamServerInterceptor(gRPCCtxTags.WithFieldExtractor(gRPCCtxTags.CodeGenRequestFieldExtractor)),
		gRPCRecovery.StreamServerInterceptor(),
		gRPCMiddleware.ChainStreamServer(s.gRPCStreamInterceptors...),
	))

	uIntOpt := grpc.UnaryInterceptor(gRPCMiddleware.ChainUnaryServer(
		gRPCCtxTags.UnaryServerInterceptor(gRPCCtxTags.WithFieldExtractor(gRPCCtxTags.CodeGenRequestFieldExtractor)),
		gRPCRecovery.UnaryServerInterceptor(),
		gRPCMiddleware.ChainUnaryServer(s.gRPCUnaryInterceptors...),
	))

	server := grpc.NewServer(sIntOpt, uIntOpt)
	s.gRPCRegister(server)
	reflection.Register(server)
	return server.Serve(l)
}

func (s *BaseGRPCService) httpServe(l net.Listener) error {
	ctx := context.Background()

	mux := runtime.NewServeMux(
		runtime.WithMarshalerOption("*", &runtime.JSONPb{
			OrigName:     true,
			EnumsAsInts:  true,
			EmitDefaults: true,
		}),
		runtime.WithMetadata(AppendRequestMetadata),
		runtime.WithForwardResponseOption(s.forwardRespFunc),
	)
	// rewrite error response
	runtime.HTTPError = TransformErrors

	opts := []grpc.DialOption{grpc.WithInsecure()}
	endPoint := fmt.Sprintf("localhost:%d", s.port)
	err := s.httpRegister(ctx, mux, endPoint, opts)
	if err != nil {
		return err
	}

	handlerMiddleware := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r)
	}))
	handlerMiddleware = HandleCrossOrigin(handlerMiddleware)

	// chain middleware functions
	for _, interceptor := range s.httpInterceptors {
		handlerMiddleware = interceptor(handlerMiddleware)
	}

	server := &http.Server{Handler: handlerMiddleware}

	return server.Serve(l)
}

// FormatHTTPResponse support sync cookie to response
func FormatHTTPResponse(ctx context.Context, w http.ResponseWriter, resp proto.Message) error {
	// md, _ := runtime.ServerMetadataFromContext(ctx)
	return nil
}

// AppendRequestMetadata append cookies and headers to incoming context
func AppendRequestMetadata(ctx context.Context, req *http.Request) metadata.MD {
	md := metadata.MD{}

	// Append cookies
	cookies := req.Cookies()
	for _, cookie := range cookies {
		md.Append(cookie.Name, cookie.Value)
	}

	// Append ip
	clientIP := "" //GetClientIP(req)
	md.Append("x-client-ip", clientIP)

	return md
}

// HandleCrossOrigin serve OPTIONS method for CORS policy
func HandleCrossOrigin(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
		} else {
			handler.ServeHTTP(w, r)
		}
	})
}

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
