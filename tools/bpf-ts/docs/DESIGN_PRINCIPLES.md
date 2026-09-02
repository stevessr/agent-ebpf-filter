# Design principles

1. Fail closed on unsupported syntax.
2. Keep the IR backend-neutral.
3. Prefer generated C that humans and verifier logs can inspect.
4. Bound loops, buffers and map sizes statically where possible.
5. Keep compiler tooling out of the deployed backend runtime.
