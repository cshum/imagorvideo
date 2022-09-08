package imagorvideo

import "go.uber.org/zap"

type Option func(p *Processor)

func WithDebug(debug bool) Option {
	return func(p *Processor) {
		p.Debug = debug
	}
}

func WithLogger(logger *zap.Logger) Option {
	return func(p *Processor) {
		if logger != nil {
			p.Logger = logger
		}
	}
}

func WithFallbackImage(image string) Option {
	return func(p *Processor) {
		p.FallbackImage = image
	}
}
