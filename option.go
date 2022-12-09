package imagorvideo

import "go.uber.org/zap"

// Option imagorvideo option
type Option func(p *Processor)

// WithDebug with debug option
func WithDebug(debug bool) Option {
	return func(p *Processor) {
		p.Debug = debug
	}
}

// WithLogger with logger option
func WithLogger(logger *zap.Logger) Option {
	return func(p *Processor) {
		if logger != nil {
			p.Logger = logger
		}
	}
}

// WithFallbackImage with fallback imagor option on error
func WithFallbackImage(image string) Option {
	return func(p *Processor) {
		p.FallbackImage = image
	}
}
