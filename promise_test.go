package promises

import (
	"context"
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

func TestNewPromiseContext(t *testing.T) {
	ctx := context.Background()
	promiseCtx := NewPromiseContext(ctx)

	assert.NotNil(t, promiseCtx)
	assert.NotNil(t, promiseCtx.Context)
	assert.Nil(t, promiseCtx.Err)
	assert.Nil(t, promiseCtx.Data)
	assert.Nil(t, promiseCtx.Promise)
}

func TestNewPromise(t *testing.T) {
	promiseCtx := NewPromiseContext(context.Background())
	p := New(promiseCtx, func(resolve func(*PromiseContext), reject func (error)) {
		// don't do anything here => keep promise in pending state
	})
	assert.Equal(t, Pending, p.State)
	// explicitly resolve promise (only done for testing)
	p.resolve(p.Result)
	result := p.Await()
	assert.Equal(t, promiseCtx, result)
	assert.Equal(t, Resolved, p.State)
}

func TestNewResolvedPromise(t *testing.T) {
	promiseCtx := NewPromiseContext(context.Background())
	p := Resolve(promiseCtx)
	// no explicit call to resolve()
	// still need to wait for any go func calls to return
	result := p.Await()
	assert.Equal(t, promiseCtx, result)
	assert.Equal(t, Resolved, p.State)
}

func TestNewRejectedPromise(t *testing.T) {
	err := errors.New("such-error-wow")
	promiseCtx := NewPromiseContext(context.Background())
	p := Reject(promiseCtx, err)
	// no explicit call to reject()
	// still need to wait for any go func calls to return
	result := p.Await()
	assert.Equal(t, promiseCtx, result)
	assert.Equal(t, Rejected, p.State)
	assert.Equal(t, err, p.Result.Err)
}

func TestPromiseThenChainResolvesPromise(t *testing.T) {
	promiseCtx := NewPromiseContext(context.Background())
	p := New(promiseCtx, func(resolve func(*PromiseContext), reject func(error)) {
		// Data is intentionally left as an interface so users can stuff whatever they
		// want and pass to downstream consumers
		promiseCtx.Data = 1
		resolve(promiseCtx)
	})
	incrementFn := func(ctx *PromiseContext) *PromiseContext {
		ctx.Data = ctx.Data.(int) + 1
		return ctx
	}
	p.
		Then(incrementFn).
		Then(incrementFn).
		Then(incrementFn)
	result := p.Await()
	assert.Equal(t, promiseCtx, result)
	assert.Equal(t, Resolved, p.State)
	assert.Nil(t, p.Result.Err)
	assert.Equal(t, 4, result.Data)
}

func TestPromiseHandlesPanicDuringLaunch(t *testing.T) {
	errMessage := "very-error-much-sadness"
	expectedErrMessage := fmt.Sprintf("Panic: %s", errMessage)
	promiseCtx := NewPromiseContext(context.Background())
	shouldNotBeCalled := func(ctx *PromiseContext) *PromiseContext {
		assert.Fail(t, "should not have called this method. promise was not correctly rejected.")
		return ctx
	}
	entry := func(resolve func(*PromiseContext), reject func(error)) {
		panic(errors.New(errMessage))
	}
	p := New(promiseCtx, entry)
	p.Then(shouldNotBeCalled)
	result := p.Await()
	assert.Equal(t, promiseCtx, result)
	assert.Equal(t, Rejected, p.State)
	actualErrMessage := p.Result.Err.Error()
	splitActualErrMessages := strings.Split(actualErrMessage, "\n")
	assert.GreaterOrEqual(t, len(splitActualErrMessages), 2)
	assert.Equal(t, expectedErrMessage, splitActualErrMessages[0])
}

func TestPromiseHandlesPanicDuringChain(t *testing.T) {
	errMessage := "very-panic"
	expectedErrMessage := fmt.Sprintf("Panic: %s", errMessage)
	promiseCtx := NewPromiseContext(context.Background())
	shouldNotBeCalled := func(ctx *PromiseContext) *PromiseContext {
		assert.Fail(t, "should not have called this method. promise was not correctly rejected.")
		return ctx
	}
	p := New(promiseCtx, func(resolve func(*PromiseContext), reject func(error)) {
		// Data is intentionally left as an interface so users can stuff whatever they
		// want and pass to downstream consumers
		promiseCtx.Data = 1
		resolve(promiseCtx)
	})
	incrementFn := func(ctx *PromiseContext) *PromiseContext {
		ctx.Data = ctx.Data.(int) + 1
		return ctx
	}
	panicFn := func(ctx *PromiseContext) *PromiseContext {
		panic(errors.New(errMessage))
	}

	p.
		Then(incrementFn).
		Then(incrementFn).
		Then(incrementFn).
		Then(incrementFn).
		Then(panicFn).
		Then(incrementFn).
		Then(incrementFn).
		Then(shouldNotBeCalled)

	result := p.Await()
	assert.Equal(t, promiseCtx, result)
	assert.Equal(t, Rejected, p.State)
	assert.Equal(t, 5, result.Data)
	actualErrMessage := p.Result.Err.Error()
	splitActualErrMessages := strings.Split(actualErrMessage, "\n")
	assert.GreaterOrEqual(t, len(splitActualErrMessages), 2)
	assert.Equal(t, expectedErrMessage, splitActualErrMessages[0])
}

func TestPromiseRespectsContext(t *testing.T) {
	expectErrMessage := "context canceled"
	ctx, cancel := context.WithCancel(context.Background())
	promiseCtx := NewPromiseContext(ctx)
	shouldNotBeCalled := func(ctx *PromiseContext) *PromiseContext {
		assert.Fail(t, "should not have called this method. promise was not correctly rejected.")
		return ctx
	}
	entry := func(resolve func(*PromiseContext), reject func(error)) {
		// simulate context cancel occurs during entry of promise chain
		cancel()
		resolve(promiseCtx)
	}
	p := New(promiseCtx, entry)
	p.Then(shouldNotBeCalled)
	result := p.Await()
	assert.Equal(t, promiseCtx, result)
	assert.Equal(t, Rejected, p.State)
	assert.Equal(t, expectErrMessage, p.Result.Err.Error())
}