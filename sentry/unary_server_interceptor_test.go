package sentrygrpc_test

import (
	"context"
	"log"
	"net"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	sentrygrpc "github.com/shizai-inc/go-grpc-interceptors/sentry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpchealth "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

var conn *grpc.ClientConn

type healthServer struct {
	grpchealth.UnimplementedHealthServer

	handler func(
		context.Context,
		*grpchealth.HealthCheckRequest,
	) (*grpchealth.HealthCheckResponse, error)
}

func (m *healthServer) Check(
	ctx context.Context,
	req *grpchealth.HealthCheckRequest,
) (*grpchealth.HealthCheckResponse, error) {
	return m.handler(ctx, req)
}

func setUp(
	handler func(context.Context, *grpchealth.HealthCheckRequest) (
		*grpchealth.HealthCheckResponse, error,
	),
	option []sentrygrpc.Option,
) {
	server := grpc.NewServer(
		grpc.UnaryInterceptor(sentrygrpc.UnaryServerInterceptor(option...)),
	)
	grpchealth.RegisterHealthServer(
		server,
		&healthServer{handler: handler},
	)

	listener := bufconn.Listen(1024 * 1024)
	go func() {
		if err := server.Serve(listener); err != nil {
			log.Fatalf("grpc serve error: %s", err)
		}
	}()

	var err error
	conn, err = grpc.DialContext(
		context.Background(),
		"bufnet",
		grpc.WithContextDialer(
			func(context.Context, string) (net.Conn, error) {
				return listener.Dial()
			},
		),
		grpc.WithInsecure(),
	)
	if err != nil {
		log.Fatalf("Failed to dial bufnet: %v", err)
	}
}

func tearDown() {
	defer conn.Close()
}

func TestUnaryServerInterceptor(t *testing.T) {
	for _, tt := range []struct {
		name    string
		ctx     context.Context
		opts    []sentrygrpc.Option
		handler func(
			context.Context,
			*grpchealth.HealthCheckRequest,
		) (*grpchealth.HealthCheckResponse, error)
		code   codes.Code
		events []*sentry.Event
	}{
		{
			name: "does not report when handler does not return error",
			ctx:  context.Background(),
			handler: func(
				context.Context,
				*grpchealth.HealthCheckRequest,
			) (*grpchealth.HealthCheckResponse, error) {
				return &grpchealth.HealthCheckResponse{}, nil
			},
			code:   codes.OK,
			events: nil,
		},
		{
			name: "reports when handler returns error",
			ctx:  context.Background(),
			handler: func(
				context.Context,
				*grpchealth.HealthCheckRequest,
			) (*grpchealth.HealthCheckResponse, error) {
				return nil, status.Error(codes.NotFound, "not found")
			},
			code: codes.NotFound,
			events: []*sentry.Event{
				{
					Level: sentry.LevelError,
					Exception: []sentry.Exception{
						{
							Type:  "*status.Error",
							Value: "rpc error: code = NotFound desc = not found",
						},
					},
				},
			},
		},
		{
			name: "does not report when report flag is false",
			ctx:  context.Background(),
			opts: []sentrygrpc.Option{
				sentrygrpc.Report(false),
			},
			handler: func(
				context.Context,
				*grpchealth.HealthCheckRequest,
			) (*grpchealth.HealthCheckResponse, error) {
				return nil, status.Error(codes.NotFound, "not found")
			},
			code:   codes.NotFound,
			events: nil,
		},
		{
			name: "sets hub on context",
			ctx:  context.Background(),
			handler: func(
				ctx context.Context,
				_ *grpchealth.HealthCheckRequest,
			) (*grpchealth.HealthCheckResponse, error) {
				if !sentry.HasHubOnContext(ctx) {
					t.Fatal("context must have hub")
				}

				return &grpchealth.HealthCheckResponse{}, nil
			},
			code: codes.OK,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var events []*sentry.Event

			if err := sentry.Init(sentry.ClientOptions{
				BeforeSend: func(
					event *sentry.Event,
					hint *sentry.EventHint,
				) *sentry.Event {
					events = append(events, event)
					return event
				},
			}); err != nil {
				log.Fatalf("sentry.Init error: %s", err)
			}

			setUp(tt.handler, tt.opts)
			defer tearDown()

			client := grpchealth.NewHealthClient(conn)

			req := &grpchealth.HealthCheckRequest{}
			_, err := client.Check(tt.ctx, req)

			if w, g := tt.code, status.Code(err); w != g {
				t.Fatalf("status mismatch: want %s, got %s", w, g)
			}

			if !sentry.Flush(time.Second) {
				t.Fatal("sentry.Flush timed out")
			}

			opts := cmp.Options{
				cmpopts.IgnoreFields(
					sentry.Event{},
					"Contexts",
					"EventID",
					"Extra",
					"Platform",
					"Sdk",
					"ServerName",
					"Tags",
					"Timestamp",
				),
				cmpopts.IgnoreFields(
					sentry.Exception{},
					"Stacktrace",
				),
			}

			if d := cmp.Diff(tt.events, events, opts); d != "" {
				t.Fatalf("events mismatch (-want +got):\n%s", d)
			}
		})
	}
}
