package query

// Predicate[T] is a composable boolean test over a value of type T.
type Predicate[T any] func(T) bool

// And returns a new Predicate that passes only when both p and other pass.
func (p Predicate[T]) And(other Predicate[T]) Predicate[T] {
	return func(v T) bool { return p(v) && other(v) }
}

// Or returns a new Predicate that passes when either p or other passes.
func (p Predicate[T]) Or(other Predicate[T]) Predicate[T] {
	return func(v T) bool { return p(v) || other(v) }
}
