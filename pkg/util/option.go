package util

// Option is a generic interface for applying configuration to a target.
// This provides a type-safe way to build functional options for any configuration type.
//
// Example:
//
//	type MyConfig struct {
//	    Name string
//	    Value int
//	}
//
//	type MyOption = util.Option[MyConfig]
//
//	func WithName(name string) MyOption {
//	    return util.FunctionalOption[MyConfig](func(cfg *MyConfig) {
//	        cfg.Name = name
//	    })
//	}
type Option[T any] interface {
	ApplyTo(target *T)
}

// FunctionalOption wraps a function to implement the Option interface.
// This allows simple functions to be used as options without defining custom types.
//
// Example:
//
//	func WithValue(val int) util.Option[MyConfig] {
//	    return util.FunctionalOption[MyConfig](func(cfg *MyConfig) {
//	        cfg.Value = val
//	    })
//	}
type FunctionalOption[T any] func(*T)

// ApplyTo implements the Option interface for FunctionalOption.
func (f FunctionalOption[T]) ApplyTo(target *T) {
	f(target)
}

// ApplyOptions applies a list of options to the target configuration.
func ApplyOptions[T any](target *T, opts ...Option[T]) {
	for _, opt := range opts {
		opt.ApplyTo(target)
	}
}
