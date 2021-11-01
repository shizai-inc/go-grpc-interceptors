package sentrygrpc

var (
	defaultOptions = &options{
		report: true,
	}
)

type Option func(*options)

type options struct {
	report bool
}

func evaluateOptions(opts ...Option) *options {
	optCopy := &options{}
	*optCopy = *defaultOptions
	for _, o := range opts {
		o(optCopy)
	}
	return optCopy
}

// Pass false when you don't want to report error to sentry(e.g. local environment)
func Report(report bool) Option {
	return func(o *options) {
		o.report = report
	}
}
