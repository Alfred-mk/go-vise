# festive: A Constrained Size Output Virtual Machine

An attempt at defining a small VM to create a stack machine for size-constrained clients and servers.

Original motivation was to create a simple templating renderer for USSD clients, combined with an agnostic data-retrieval reference that may conceal any level of complexity.


## Opcodes

The VM defines the following opcode symbols:

* `BACK` - Return to the previous execution frame (will fail if at top frame). It leaves to the state of the execution layer to define what "previous" means.
* `CATCH <symbol> <signal>` - Jump to symbol if signal is set (see `signal` below).
* `CROAK <signal>` - Clear state and restart execution from top if signal is set (see `signal` below).
* `LOAD <symbol> <size>` - Execute the code symbol `symbol` and cache the data, constrained to the given `size`. Can be exposed with `MAP` within scope, 
* `RELOAD <symbol>` - Execute a code symbol already loaded by `LOAD` and cache the data, constrained to the previously given `size` for the same symbol. 
* `MAP <symbol>` - Expose a code symbol previously loaded by `LOAD` to the rendering client. Roughly corresponds to the `global` directive in Python.
* `MOVE <symbol>` - Create a new execution frame, invalidating all previous `MAP` calls. More detailed: After a `MOVE` call, a `BACK` call will return to the same execution frame, with the same symbols available, but all `MAP` calls will have to be repeated.


### External code

`LOAD` is used to execute code symbols in the host environment. It is loaded with a size constraint, and returned data violating this constraint should generate an error.

Any symbol successfully loaded with `LOAD` will be associated with the call stack frame it is loaded. The symbol will be available in the same frame and frames below it. Once the frame goes out of scope (e.g. `BACK` is called in that frame) the symbols should be freed as soon as possible. At this point they are not available to the abandoned scope.

Loaded symbols are not automatically exposed to the rendering client. To expose symbols ot the rendering client the `MAP` opcode must be used.

The associated content of loaded symbols may be refreshed using the `RELOAD` opcode. `RELOAD` only works within the same constraints as `MAP`. However, updated content must be available even if a `MAP` precedes a `RELOAD` within the same frame.

### External symbol optimizations

Only `LOAD` and `RELOAD` should trigger external code side-effects. 

In an effort to prevent leaks from unnecessary external code executions, the following constraints are assumed:

- An explicit `MAP` **must** exist in the scope of any `LOAD`.
- All symbols declared in `MAP` **must** be used for all template renderings of a specific node.

Any code compiler or checked **should** generate an error on any orphaned `LOAD` or `MAP` symbols as described above.


## Rendering

The fixed-size output is generated using a templating language, and a combination of one or more _max size_ properties, and an optional _sink_ property that will attempt to consume all remaining capacity of the rendered template.

For example, in this example

- `maxOutputSize` is 256 bytes long.
- `template` is 120 bytes long.
- param `one` has max size 10 but uses 5.
- param `two` has max size 20 but uses 12.
- param `three` is a _sink_.

The renderer may use up to `256 - 120 - 5 - 12 = 119` bytes from the _sink_ when rendering the output.


### Multipage support

Multipage outputs, like listings, are handled using the _sink_ output constraints:

- first calculate what the rendered display size is when all symbol results that are _not_ sinks are resolved.
- split and cache the list data within its semantic context, given the _sink_ limitation after rendering.
- provide a `next` and `previous` menu item to browse the prepared pagination of the list data.


### Languages support

Language for rendering is determined at the top-level state.

Lookups dependent on language are prefixed by either `ISO 639-1` or `ISO 639-3` language codes, followed by `:`.

Default language means records returned without prefix if no language is set. Default language should be settable at the top-level.

Node names **must** be defined using 7-bit ASCII.


## Virtual machine interface layout

This is the version `0` of the VM. That translates to  _highly experimental_.

Currently the following rules apply for encoding in version `0`:

- A code instruction is a _big-endian_ 2-byte value. See `vm/opcodes.go` for valid opcode values.
- `symbol` value is encoded as _one byte_ of string length, after which the  byte-value of the string follows.
- `size` value is encoded as _one byte_ of numeric length, after which the _big-endian_ byte-value of the integer follows.
- `signal` value is encoded as _one byte_ of byte length, after which a byte-array representing the defined signal follows.


## Reference implementation

This repository provides a `golang` reference implementation for the `festive` concept.

In this reference implementation some constraints apply


### Structure

_TODO_: `state` will be separated into `cache` and `session`.

- `vm`: Defines instructions, and applies transformations according to the instructions.
- `state`: Holds the code cache, contents cache aswell as error tates from code execution.
- `resource`: Retrieves data and bytecode from external symbols, and retrieves and renders templates.
- `engine`: Outermost interface. Orchestrates execution of bytecode against input. 


### Template rendering

Template rendering is done using the `text/template` faciilty in the `golang` standard library. 

It expects all replacement symbols to be available at time of rendering, and has no tolerance for missing ones.


## Assembly language

**TBD**

An assmebly language will be defined to generate the _routing_ and _execution_ bytecodes for each menu node.
