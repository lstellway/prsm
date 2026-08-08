package query

// Predicate[T] is a composable boolean test over a value of type T.
type Predicate[T any] func(T) bool

// And returns a new Predicate that passes only when both predicate and other pass.
func (predicate Predicate[T]) And(other Predicate[T]) Predicate[T] {
	return func(value T) bool { return predicate(value) && other(value) }
}

// Or returns a new Predicate that passes when either predicate or other passes.
func (predicate Predicate[T]) Or(other Predicate[T]) Predicate[T] {
	return func(value T) bool { return predicate(value) || other(value) }
}
