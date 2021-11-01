package sentrygrpc

import (
	"context"

	"github.com/getsentry/sentry-go"
	"google.golang.org/grpc"
)

// TODO: Use the official interceptor once https://github.com/getsentry/sentry-go/pull/312
//       is released
//
// Usage:
//
//     grpcServer := grpc.NewServer(
//	       grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(
//             grpc_recovery.UnaryServerInterceptor(),
//             interceptors.SentryUnaryServerInterceptor(),
//         )),
//     )
//
// As above, you would always want to pass this interceptors in the latter part
// so `RecoverWithContext` and `CaptureException` gets executed as fast as possible
// inside the interceptor chain.(defer and logics after `handler` are executed reversibly)
//
func UnaryServerInterceptor(options ...Option) grpc.UnaryServerInterceptor {
	opts := evaluateOptions(options...)

	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		hub := sentry.GetHubFromContext(ctx)
		if hub == nil {
			hub = sentry.CurrentHub().Clone()
			ctx = sentry.SetHubOnContext(ctx, hub)
		}

		defer func() {
			if r := recover(); r != nil {
				if opts.report {
					hub.RecoverWithContext(ctx, r)
				}

				panic(r)
			}
		}()

		resp, err = handler(ctx, req)

		if err != nil && opts.report {
			hub.CaptureException(err)
		}

		return resp, err
	}
}
