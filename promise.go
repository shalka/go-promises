package promises

import (
	"context"
	"fmt"
	"runtime"
	"sync"
)

// PromiseContext wraps the information passed between calls in the promise chain.
type PromiseContext struct {
	// Context provides mechanism for caller to abort promise execution
	Context context.Context

	// Data provides mechanism for passing data between chained callbacks
	Data interface{}

	// Stores the error passed to reject()
	Err error

	// Promise holds a pointer back to the containing Promise
	Promise *Promise
}

type PromiseState string

const (
	Pending  PromiseState = "Pending"
	Resolved PromiseState = "Resolved"
	Rejected PromiseState = "Rejected"

	StackBufferSize int = 1024
)

// NewPromiseContext is a convenience method for initializing a PromiseContext using a golang context
func NewPromiseContext(ctx context.Context) *PromiseContext {
	return &PromiseContext{
		Context: ctx,
		Data:    nil,
		Err:     nil,
		Promise: nil,
	}
}

// A Promise, or CompletableFuture, allows us to chain asynchronous callbacks with a
//   guarantee that all calculations will eventually be completed.
type Promise struct {
	// State is one of: [Pending, Resolved, or Rejected]
	State PromiseState

	// Entry provides an initial point of entry to the promise chain.
	//   The resolve method will resolve the Promise (updating State to Resolved) when called.
	//   The reject  method will reject  the Promise (updating State to Rejected) when called.
	Entry func(resolve func(*PromiseContext), reject func(error))

	// Stores the result passed to resolve()
	Result *PromiseContext

	// Keeps track of which .Then methods have been executed
	executed []bool

	// then is a list of callbacks representing the promise chain.
	then []func(*PromiseContext) *PromiseContext

	// mutex provides synchronization primitive
	mutex *sync.Mutex

	// waiter blocks until all callbacks are executed
	waiter *sync.WaitGroup
}

// New instantiates and returns a pointer to the Promise.
func New(ctx *PromiseContext, entry func(resolve func(*PromiseContext), reject func(error))) *Promise {
	var wg = &sync.WaitGroup{}
	wg.Add(1)

	var p = &Promise{
		State:  Pending,
		Entry:  entry,
		Result: ctx,
		then:   []func(*PromiseContext) *PromiseContext{},
		mutex:  &sync.Mutex{},
		waiter: wg,
	}
	p.Result.Promise = p

	go func() {
		// mutex is only locked during .Then chain, don't try to unlock mutex
		// if we need to handle a panic during initial entry to promise
		defer p.handlePanic(false)
		p.Entry(p.resolve, p.reject)
	}()

	return p
}

// Await blocks for all callbacks to be executed. Call on a Resolved Promise to get its result
func (p *Promise) Await() *PromiseContext {
	p.waiter.Wait()
	return p.Result
}

// stolen shamelessly from: https://github.com/oracle/oci-go-sdk/blob/master/common/retry.go
func (p *Promise) handlePanic(shouldUnlockMutex bool) {
	if r := recover(); r != nil {
		buffer := make([]byte, StackBufferSize)
		numBytes := runtime.Stack(buffer, false)
		stack := string(buffer[:numBytes])
		if shouldUnlockMutex {
			p.mutex.Unlock()
		} else {
			// this handles a particularly nefarious race where the p.Entry go func doesn't
			//  resolve before a .Then() is registered and a subsequent panic occurs
			p.waiter.Done()
		}
		p.reject(fmt.Errorf("Panic: %s\nStack: %s", r, stack))
	}
}

func (p *Promise) resolve(data *PromiseContext) {
	p.mutex.Lock()

	if p.State != Pending {
		p.mutex.Unlock()
		return
	}
	p.Result = data
	p.waiter.Done()

	if err := p.Result.Context.Err(); err != nil {
		p.mutex.Unlock()
		p.reject(err)
		return
	}

	next := make(chan *PromiseContext)

	for idx, fn := range p.then {
		go func() {
			defer p.handlePanic(true)
			next <- fn(p.Result)
		}()
		select {
		case <-p.Result.Context.Done():
			// reject promise and break promise chain
			p.mutex.Unlock()
			p.reject(p.Result.Context.Err())
			return
		case outcome := <-next:
			// this block represents successful resolution of the invoked method.. now
			//  we need to check to ensure that err wasn't populated and update internal
			//  state of the promise to reflect the outcome.
			result, err := outcome.Promise.Result, outcome.Err
			if err != nil {
				p.mutex.Unlock()
				p.reject(err)
				return
			}
			p.Result = result
			p.executed[idx] = true
			p.waiter.Done()
		}
	}
	p.State = Resolved
	p.mutex.Unlock()
}

func (p *Promise) reject(err error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.State != Pending {
		return
	}

	p.Result.Err = err

	// this is necessary for promises that call Reject directly or fail during initial launch
	//  of promise chain (introducing an additional operators into the reject method signature
	//  would pollute the calling pattern and leak state to end users, so unfortunately we're
	//  using size of p.executed as our diving rod for whether or not we need to call .Done()
	//  on the promise's WaitGroup).
	if len(p.executed) == 0 {
		p.waiter.Done()
	}

	for _, isThenExecuted := range p.executed {
		if !isThenExecuted {
			p.waiter.Done()
		}
	}
	p.State = Rejected
}

// Then appends fulfillment handler to the Promise, and returns the Promise (useful for chaining readability).
func (p *Promise) Then(op func(ctx *PromiseContext) *PromiseContext) *Promise {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.State == Pending {
		p.waiter.Add(1)
		p.then = append(p.then, op)
		p.executed = append(p.executed, false)
	} else if p.State == Resolved {
		outcome := op(p.Result)
		if result, err := outcome.Promise.Result, outcome.Err; err != nil {
			p.reject(err)
		} else {
			p.Result = result
		}
	}
	// ignore the asynchronous operation if the promise has already Rejected
	return p
}

// Resolve returns a Promise that has been resolved with the specified context
func Resolve(context *PromiseContext) *Promise {
	return New(context, func(resolve func(*PromiseContext), reject func(error)) {
		resolve(context)
	})
}

// Reject returns a Promise that has been rejected with a given error
func Reject(context *PromiseContext, err error) *Promise {
	return New(context, func(resolve func(*PromiseContext), reject func(error)) {
		reject(err)
	})
}
