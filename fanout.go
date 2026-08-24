package prsm

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lstellway/prsm/adapter"
	"github.com/lstellway/prsm/model"
)

// fanOut calls call once per source in sources, concurrently, and returns one
// result and one status per source, in sources order. It is the concurrency
// skeleton Client.Fetch and Client.ResolveIdentities share: pre-size two
// result slices so each goroutine only ever writes its own index, launch one
// goroutine per source with a deferred recover so one connection panicking
// cannot take the others down or leave the caller's WaitGroup hanging
// forever, and hand call's (value, error) to newStatus to build whatever
// status shape the caller needs — a ConnectionStatus for Fetch, an
// IdentityStatus for ResolveIdentities.
//
// newStatus's time.Time argument is the moment call returned for that
// source; a status type that only records the moment on success (mirroring
// ConnectionStatus.SucceededAt and IdentityStatus.ResolvedAt) is free to
// ignore it on the error path, so the panic branch below can pass
// time.Now() rather than threading a distinguished start time through.
func fanOut[S adapter.Connection, R any, ST any](
	ctx context.Context,
	sources []S,
	call func(context.Context, S) (R, error),
	newStatus func(model.ProviderInstance, time.Time, error) ST,
) ([]R, []ST) {
	results := make([]R, len(sources))
	statuses := make([]ST, len(sources))

	var waitGroup sync.WaitGroup
	for index, source := range sources {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					statuses[index] = newStatus(source.Instance(), time.Now(), fmt.Errorf("panic: %v", recovered))
				}
			}()

			result, err := call(ctx, source)
			returnedAt := time.Now()
			results[index] = result
			statuses[index] = newStatus(source.Instance(), returnedAt, err)
		}()
	}
	waitGroup.Wait()

	return results, statuses
}
