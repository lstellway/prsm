package model

// LoadState is the fetch lifecycle of a lazy-loaded field.
type LoadState uint8

const (
	LoadStatePending LoadState = iota // zero value: not yet fetched
	LoadStateLoaded                   // fetched successfully; value is valid
	LoadStateAbsent                   // provider does not expose this field
	LoadStateError                    // fetch was attempted but failed
)

// LoadResult represents a field that may not yet have been fetched, may have a value,
// or may be absent because the provider does not supply it.
// The zero value is LoadStatePending.
type LoadResult[T any] struct {
	state LoadState
	value T
	err   error
}

// Pending returns a LoadResult in the not-yet-fetched state.
func Pending[T any]() LoadResult[T] { return LoadResult[T]{state: LoadStatePending} }

// Loaded returns a LoadResult with a successfully fetched value.
func Loaded[T any](v T) LoadResult[T] { return LoadResult[T]{state: LoadStateLoaded, value: v} }

// Absent returns a LoadResult indicating the provider does not expose this field.
func Absent[T any]() LoadResult[T] { return LoadResult[T]{state: LoadStateAbsent} }

// Failed returns a LoadResult indicating a fetch was attempted but failed.
func Failed[T any](cause error) LoadResult[T] { return LoadResult[T]{state: LoadStateError, err: cause} }

func (r LoadResult[T]) IsPending() bool { return r.state == LoadStatePending }
func (r LoadResult[T]) IsLoaded() bool  { return r.state == LoadStateLoaded }
func (r LoadResult[T]) IsAbsent() bool  { return r.state == LoadStateAbsent }
func (r LoadResult[T]) IsError() bool   { return r.state == LoadStateError }

// Err returns the error cause if the result is in the Failed state, otherwise nil.
func (r LoadResult[T]) Err() error { return r.err }

// Get returns the value and true if loaded, otherwise the zero value and false.
func (r LoadResult[T]) Get() (T, bool) { return r.value, r.state == LoadStateLoaded }

// MustGet returns the value or panics if not in the Loaded state.
func (r LoadResult[T]) MustGet() T {
	if r.state != LoadStateLoaded {
		panic("LoadResult.MustGet called on non-Loaded result")
	}
	return r.value
}

// UnwrapOr returns the value if loaded, otherwise def.
func (r LoadResult[T]) UnwrapOr(def T) T {
	if r.state == LoadStateLoaded {
		return r.value
	}
	return def
}
