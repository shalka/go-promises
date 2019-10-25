# go-promises

A promise library with a focus on real-world failure semantics.

##Context

This implementation is an extension of the excellent promise library (inspired by JS Promises):
https://github.com/chebyrash/promise but also takes concepts from Martin Sulzmann's paper on Futures and Promises,
titled "From Events to Futures and Promises and back", found here:
http://www.home.hs-karlsruhe.de/~suma0002/publications/events-to-futures.pdf.

This library was inspired by a deployment tool at Grab, which follows the model for a communicating sequential process (CSP) very well. It is intended to replace a series of bash scripts where contextual information (read: global state) can be propagated through environment variables, while providing richer testability and portability.

##What's Different

* The promise receives a context.Context that is respected throughout promise chain execution
* Instead of .Catch() method, a PromiseContext is propagated through the chain call that contains an Err (error) member. If Err is non-nil after any invocation, the promise is rejected.
* The PromiseContext object contains a Data (type interface{}) member for propagating information between methods. This allows users to decide what information would be useful to downstream consumers in building richer conditional executions.
