# Simple IoT fork

Source: `github.com/simpleiot/simpleiot`, tag `v0.31.0`.
License: see `LICENSE`.

easyems keeps integration changes in this pinned tree:

- `CustomUIFS` also serves the embedded product UI at `/`.
- `AppOnly` mounts the product API at `/api` and disables generic `/v1`, `/old` and browser WebSocket proxy routes.
- `AppHandlerFactory` constructs the product handler after the store and user authority exist.
- Product-only mode disables the internal WebSocket listener and binds the NATS client listener to loopback.
- The Modbus client publishes `lastGoodPoll` after a complete successful polling cycle so easyems can reject stale automatic-control inputs.
- Point history uses an ordered ephemeral JetStream consumer; creating named consumers per concurrent history query caused 503s when several points loaded at once.
- `DisableHTTP` skips the built-in HTTP listener while still constructing the application handler, so Encore can be the only listener.

These changes avoid a second HTTP service, a second business process and product-user bypass of the application boundary. The upstream API router test covers the restricted routes.
