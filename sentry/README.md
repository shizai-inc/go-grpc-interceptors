# Sentry gRPC interceptors

## Usage

```go
    sentryOpts := []sentrygrpc.Option{
        sentrygrpc.Report(false),
    }

    grpcServer := grpc.NewServer(
        grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(
            grpc_recovery.UnaryServerInterceptor(),
            sentrygrpc.SentryUnaryServerInterceptor(sentryOpts...),
        )),
    )
```
