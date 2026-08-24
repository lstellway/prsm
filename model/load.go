package model

import "encoding/json"

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
func Loaded[T any](value T) LoadResult[T] { return LoadResult[T]{state: LoadStateLoaded, value: value} }

// Absent returns a LoadResult indicating the provider does not expose this field.
func Absent[T any]() LoadResult[T] { return LoadResult[T]{state: LoadStateAbsent} }

// Failed returns a LoadResult indicating a fetch was attempted but failed.
func Failed[T any](cause error) LoadResult[T] {
	return LoadResult[T]{state: LoadStateError, err: cause}
}

func (loadResult LoadResult[T]) IsPending() bool { return loadResult.state == LoadStatePending }
func (loadResult LoadResult[T]) IsLoaded() bool  { return loadResult.state == LoadStateLoaded }
func (loadResult LoadResult[T]) IsAbsent() bool  { return loadResult.state == LoadStateAbsent }
func (loadResult LoadResult[T]) IsError() bool   { return loadResult.state == LoadStateError }

// Err returns the error cause if the result is in the Failed state, otherwise nil.
func (loadResult LoadResult[T]) Err() error { return loadResult.err }

// Get returns the value and true if loaded, otherwise the zero value and false.
func (loadResult LoadResult[T]) Get() (T, bool) {
	return loadResult.value, loadResult.state == LoadStateLoaded
}

// MustGet returns the value or panics if not in the Loaded state.
func (loadResult LoadResult[T]) MustGet() T {
	if loadResult.state != LoadStateLoaded {
		panic("LoadResult.MustGet called on non-Loaded result")
	}
	return loadResult.value
}

// UnwrapOr returns the value if loaded, otherwise fallback.
func (loadResult LoadResult[T]) UnwrapOr(fallback T) T {
	if loadResult.state == LoadStateLoaded {
		return loadResult.value
	}
	return fallback
}

func (loadState LoadState) String() string {
	switch loadState {
	case LoadStatePending:
		return "pending"
	case LoadStateLoaded:
		return "loaded"
	case LoadStateAbsent:
		return "absent"
	case LoadStateError:
		return "error"
	default:
		return "unknown"
	}
}

// MarshalJSON encodes the state as its String() name rather than the
// underlying uint8, so JSON output reads "loaded" instead of "1".
func (loadState LoadState) MarshalJSON() ([]byte, error) {
	return json.Marshal(loadState.String())
}

// loadResultJSON is the wire shape for a LoadResult[T]. Value is `any`
// rather than T so a Loaded zero value (e.g. CommentCount 0) still encodes
// as 0, while a genuinely absent value omits the field entirely — a generic
// T field could not distinguish the two, since omitempty treats every T's
// own zero value as empty.
type loadResultJSON struct {
	State LoadState `json:"state"`
	Value any       `json:"value,omitempty"`
	Error string    `json:"error,omitempty"`
}

// MarshalJSON encodes the field's fetch lifecycle explicitly rather than
// falling through to the zero-value default a plain struct tag would produce
// for value/err — both of which are unexported and would otherwise vanish
// from JSON output entirely, silently reporting every lazy-loaded field as
// an empty object regardless of its actual state.
func (loadResult LoadResult[T]) MarshalJSON() ([]byte, error) {
	document := loadResultJSON{State: loadResult.state}
	if loadResult.state == LoadStateLoaded {
		document.Value = loadResult.value
	}
	if loadResult.state == LoadStateError && loadResult.err != nil {
		document.Error = loadResult.err.Error()
	}
	return json.Marshal(document)
}
